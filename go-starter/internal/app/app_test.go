package app

import (
	"context"
	"errors"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/codingseeker/go-starter/internal/config"
	"github.com/codingseeker/go-starter/internal/domain"
	"github.com/codingseeker/go-starter/internal/logger"
	"github.com/codingseeker/go-starter/internal/service"
)

// lockedBuffer is a concurrency safe log sink, because a fake server logs from
// its own goroutine while the test reads.
type lockedBuffer struct {
	mu  sync.Mutex
	buf strings.Builder
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func testConfig() config.Config {
	cfg := config.Default()
	cfg.AppName = "app-test"
	cfg.Environment = config.EnvironmentTesting
	cfg.LogLevel = config.LogLevelDebug
	return cfg
}

// newTestApp wires an App whose signal handling is replaced by plain context
// cancellation, so tests never raise real operating system signals.
func newTestApp(t *testing.T, opts ...Option) (*App, *lockedBuffer) {
	t.Helper()

	cfg := testConfig()
	logs := &lockedBuffer{}
	log, err := logger.New(cfg, logs)
	if err != nil {
		t.Fatalf("logger.New() error = %v", err)
	}

	opts = append([]Option{WithSignalContext(passthroughSignals)}, opts...)
	return New(cfg, log, service.New(cfg), opts...), logs
}

func passthroughSignals(parent context.Context) (context.Context, context.CancelFunc) {
	return context.WithCancel(parent)
}

// cancelAfter simulates a signal arriving after a fixed delay.
func cancelAfter(d time.Duration) SignalContext {
	return func(parent context.Context) (context.Context, context.CancelFunc) {
		return context.WithTimeout(parent, d)
	}
}

// fakeServer is a controllable Server implementation.
type fakeServer struct {
	startErr    error
	shutdownErr error
	blockOnStop bool

	startCalls atomic.Int32
	shutdown   atomic.Bool
	stopped    atomic.Bool
	closeOnce  sync.Once
	shutdownCh chan struct{}
}

func newFakeServer(blockOnStop bool) *fakeServer {
	return &fakeServer{blockOnStop: blockOnStop, shutdownCh: make(chan struct{})}
}

func (f *fakeServer) Start(ctx context.Context) error {
	f.startCalls.Add(1)
	defer f.stopped.Store(true)

	if f.blockOnStop {
		select {
		case <-ctx.Done():
		case <-f.shutdownCh:
		}
	}
	return f.startErr
}

func (f *fakeServer) Shutdown(context.Context) error {
	f.shutdown.Store(true)
	f.closeOnce.Do(func() { close(f.shutdownCh) })
	return f.shutdownErr
}

func TestNewKeepsDependencies(t *testing.T) {
	cfg := config.Default()
	svc := service.New(cfg)

	a := New(cfg, nil, svc)
	if a == nil {
		t.Fatal("New() = nil")
	}
	if a.Config() != cfg {
		t.Errorf("Config() = %+v, want %+v", a.Config(), cfg)
	}
	if a.Service() != svc {
		t.Error("Service() must return the injected service")
	}
	if a.signalContext == nil {
		t.Error("signalContext must default to real signal handling")
	}
	if a.logger == nil {
		t.Error("a nil logger must be replaced by a no-op logger")
	}
}

func TestNewIgnoresEmptyOptions(t *testing.T) {
	a := New(config.Default(), logger.Noop(), service.New(config.Default()),
		nil, WithCloser(nil), WithSignalContext(nil))

	if a.service == nil || a.logger == nil || a.signalContext == nil {
		t.Fatal("empty options must not clear the defaults")
	}
	if len(a.closers) != 0 {
		t.Errorf("closers = %d, want 0", len(a.closers))
	}
}

func TestRunStopsOnContextCancellation(t *testing.T) {
	application, logs := newTestApp(t)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- application.Run(ctx) }()

	waitForLog(t, logs, "application started")
	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run() error = %v, want nil", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Run() did not return after the context was cancelled")
	}

	waitForLog(t, logs, "application stopped")
}

func TestRunUsesTheInjectedService(t *testing.T) {
	application, logs := newTestApp(t, WithSignalContext(cancelAfter(time.Millisecond)))
	recorder := &recordingService{}

	application.service = recorder
	if err := application.Run(context.Background()); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if recorder.calls.Load() != 1 {
		t.Errorf("service used %d times, want 1", recorder.calls.Load())
	}
	if recorder.lastName != testConfig().AppName {
		t.Errorf("service received name %q, want %q", recorder.lastName, testConfig().AppName)
	}
	if !strings.Contains(logs.String(), "greeting=") {
		t.Errorf("the result of the work must be logged, got:\n%s", logs.String())
	}
}

func TestRunDoesNotLeakGoroutines(t *testing.T) {
	before := runtime.NumGoroutine()

	for i := 0; i < 25; i++ {
		application, _ := newTestApp(t, WithSignalContext(cancelAfter(time.Millisecond)))
		if err := application.Run(context.Background()); err != nil {
			t.Fatalf("Run() error = %v", err)
		}
	}

	assertNoGoroutineLeak(t, before)
}

func TestRunReleasesResourcesOnce(t *testing.T) {
	var released atomic.Int32
	application, logs := newTestApp(t,
		WithSignalContext(cancelAfter(5*time.Millisecond)),
		WithCloser(func(context.Context) error {
			released.Add(1)
			return nil
		}),
	)

	if err := application.Run(context.Background()); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if released.Load() != 1 {
		t.Errorf("closer called %d times, want 1", released.Load())
	}
	waitForLog(t, logs, "resources released")
}

func TestRunReportsCloserFailure(t *testing.T) {
	closerErr := errors.New("closer failed")
	application, _ := newTestApp(t,
		WithSignalContext(cancelAfter(5*time.Millisecond)),
		WithCloser(func(context.Context) error { return closerErr }),
	)

	if err := application.Run(context.Background()); !errors.Is(err, closerErr) {
		t.Fatalf("Run() error = %v, want it to wrap the closer failure", err)
	}
}

func TestRunPropagatesServiceErrors(t *testing.T) {
	application, _ := newTestApp(t)
	application.service = failingService{}

	err := application.Run(context.Background())
	if err == nil {
		t.Fatal("Run() error = nil, want the service failure")
	}
	if !errors.Is(err, errFailingService) {
		t.Errorf("Run() error = %v, want it to wrap the service error", err)
	}
}

func TestRunRequiresService(t *testing.T) {
	application := New(config.Default(), logger.Noop(), nil)
	if err := application.Run(context.Background()); err == nil {
		t.Fatal("Run() error = nil, want a configuration error")
	}
}

func TestRunTreatsCancellationAsAStop(t *testing.T) {
	application, _ := newTestApp(t)
	application.service = failingService{}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if err := application.Run(ctx); err != nil {
		t.Errorf("Run() error = %v, want nil: a cancelled context is a requested stop", err)
	}
}

func TestRunReleasesResourcesWhenCancelledBeforeTheWork(t *testing.T) {
	application, _ := newTestApp(t)
	application.service = failingService{}

	var released int
	application.closers = append(application.closers, func(context.Context) error {
		released++
		return nil
	})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if err := application.Run(ctx); err != nil {
		t.Fatalf("Run() error = %v, want nil", err)
	}
	if released != 1 {
		t.Errorf("closers called %d times, want exactly 1", released)
	}
}

func TestServeGracefulShutdown(t *testing.T) {
	application, logs := newTestApp(t)
	srv := newFakeServer(true)

	// Registered in the same order as the CLI does: drain state first, then
	// the transport itself.
	order := make([]string, 0, 2)
	application.closers = append(application.closers,
		func(context.Context) error { order = append(order, "drain"); return nil },
		func(context.Context) error {
			order = append(order, "server")
			return srv.Shutdown(context.Background())
		},
	)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- application.Serve(ctx, srv) }()

	waitForStart(t, srv)
	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Serve() error = %v, want nil", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Serve() did not return after cancellation")
	}

	if !srv.shutdown.Load() {
		t.Error("the server must have been asked to shut down")
	}
	if strings.Join(order, ",") != "drain,server" {
		t.Errorf("closers ran in order %v, want [drain server]", order)
	}
	waitForLog(t, logs, "resources released")
	srv.waitStopped(t)
}

func TestServeStartupFailure(t *testing.T) {
	startErr := errors.New("address already in use")
	application, logs := newTestApp(t)
	srv := &fakeServer{startErr: startErr, shutdownCh: make(chan struct{})}

	var shutdownCalls atomic.Int32
	application.closers = append(application.closers, func(context.Context) error {
		shutdownCalls.Add(1)
		return srv.Shutdown(context.Background())
	})

	err := application.Serve(context.Background(), srv)
	if err == nil {
		t.Fatal("Serve() error = nil, want the startup failure")
	}
	if !errors.Is(err, startErr) {
		t.Errorf("Serve() error = %v, want it to wrap the startup failure", err)
	}
	waitForLog(t, logs, "server stopped unexpectedly")
	if shutdownCalls.Load() != 1 {
		t.Errorf("resources released %d times, want 1 after a startup failure", shutdownCalls.Load())
	}
}

func TestServeShutdownFailure(t *testing.T) {
	shutdownErr := errors.New("shutdown timed out")
	application, _ := newTestApp(t)
	srv := newFakeServer(true)
	srv.shutdownErr = shutdownErr

	var shutdownCalls atomic.Int32
	application.closers = append(application.closers, func(context.Context) error {
		shutdownCalls.Add(1)
		return srv.Shutdown(context.Background())
	})

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- application.Serve(ctx, srv) }()
	waitForStart(t, srv)
	cancel()

	select {
	case err := <-done:
		if !errors.Is(err, shutdownErr) {
			t.Errorf("Serve() error = %v, want it to wrap the shutdown failure", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Serve() did not return after cancellation")
	}
	if shutdownCalls.Load() != 1 {
		t.Errorf("the server shutdown was attempted %d times, want 1", shutdownCalls.Load())
	}
}

func TestServeJoinsShutdownAndStopErrors(t *testing.T) {
	shutdownErr := errors.New("shutdown failed")
	startErr := errors.New("start returned an error")
	application, _ := newTestApp(t)
	srv := newFakeServer(true)
	srv.startErr = startErr
	srv.shutdownErr = shutdownErr

	application.closers = append(application.closers, srv.Shutdown)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- application.Serve(ctx, srv) }()
	waitForStart(t, srv)
	cancel()

	select {
	case err := <-done:
		if !errors.Is(err, shutdownErr) || !errors.Is(err, startErr) {
			t.Errorf("Serve() error = %v, want it to join both failures", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Serve() did not return after cancellation")
	}
}

func TestServeStopsWhenServerExitsImmediately(t *testing.T) {
	application, logs := newTestApp(t)
	srv := newFakeServer(false)

	if err := application.Serve(context.Background(), srv); err != nil {
		t.Fatalf("Serve() error = %v, want nil when the server stops cleanly", err)
	}
	waitForLog(t, logs, "resources released")
}

func TestServeDoesNotLeakGoroutines(t *testing.T) {
	before := runtime.NumGoroutine()

	for i := 0; i < 25; i++ {
		application, _ := newTestApp(t, WithSignalContext(cancelAfter(2*time.Millisecond)))
		srv := newFakeServer(true)
		application.closers = append(application.closers, srv.Shutdown)
		if err := application.Serve(context.Background(), srv); err != nil {
			t.Fatalf("Serve() error = %v", err)
		}
		if !srv.stopped.Load() {
			t.Fatal("the Start goroutine outlived Serve")
		}
	}

	assertNoGoroutineLeak(t, before)
}

func TestServeRequiresServer(t *testing.T) {
	application, _ := newTestApp(t)
	if err := application.Serve(context.Background(), nil); err == nil {
		t.Fatal("Serve() error = nil, want a configuration error")
	}
}

func TestNotifyContextFollowsParentCancellation(t *testing.T) {
	parent, cancel := context.WithCancel(context.Background())
	ctx, stop := NotifyContext(parent)
	defer stop()

	cancel()
	select {
	case <-ctx.Done():
	case <-time.After(time.Second):
		t.Fatal("NotifyContext did not follow the parent cancellation")
	}
}

func TestIsExpectedStop(t *testing.T) {
	if !isExpectedStop(context.Canceled) || !isExpectedStop(context.DeadlineExceeded) {
		t.Error("context errors must be treated as expected stops")
	}
	if isExpectedStop(errors.New("boom")) {
		t.Error("unrelated errors must not be treated as expected stops")
	}
}

var errFailingService = errors.New("service failure")

type failingService struct{}

func (failingService) Health(context.Context) (domain.HealthStatus, error) {
	return domain.HealthStatus{}, errFailingService
}

func (failingService) Ready(context.Context) (domain.HealthStatus, error) {
	return domain.HealthStatus{}, errFailingService
}

func (failingService) Greet(context.Context, string) (domain.Greeting, error) {
	return domain.Greeting{}, errFailingService
}

type recordingService struct {
	calls    atomic.Int32
	lastName string
}

func (r *recordingService) Health(context.Context) (domain.HealthStatus, error) {
	return domain.HealthStatus{Status: domain.StatusOK}, nil
}

func (r *recordingService) Ready(context.Context) (domain.HealthStatus, error) {
	return domain.HealthStatus{Status: domain.StatusOK}, nil
}

func (r *recordingService) Greet(_ context.Context, name string) (domain.Greeting, error) {
	r.calls.Add(1)
	r.lastName = name
	return domain.Greeting{Name: name, Message: "hello " + name}, nil
}

func (f *fakeServer) waitStopped(t *testing.T) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for !f.stopped.Load() {
		if time.Now().After(deadline) {
			t.Fatal("the Start goroutine did not return after shutdown")
		}
		time.Sleep(time.Millisecond)
	}
}

func waitForStart(t *testing.T, srv *fakeServer) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for srv.startCalls.Load() == 0 {
		if time.Now().After(deadline) {
			t.Fatal("Start was never called")
		}
		time.Sleep(time.Millisecond)
	}
}

func waitForLog(t *testing.T, logs *lockedBuffer, want string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for !strings.Contains(logs.String(), want) {
		if time.Now().After(deadline) {
			t.Fatalf("log entry %q not found in:\n%s", want, logs.String())
		}
		time.Sleep(time.Millisecond)
	}
}

func assertNoGoroutineLeak(t *testing.T, before int) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		after := runtime.NumGoroutine()
		if after <= before+2 {
			return
		}
		if time.Now().After(deadline) {
			stacks := make([]byte, 1<<16)
			stacks = stacks[:runtime.Stack(stacks, true)]
			t.Fatalf("goroutine count grew from %d to %d:\n%s", before, after, stacks)
		}
		time.Sleep(5 * time.Millisecond)
	}
}
