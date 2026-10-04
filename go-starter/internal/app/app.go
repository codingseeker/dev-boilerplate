// Package app owns the application lifecycle: dependency wiring, signal
// handling, controlled startup and graceful shutdown.
package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/codingseeker/go-starter/internal/config"
	"github.com/codingseeker/go-starter/internal/domain"
	"github.com/codingseeker/go-starter/internal/logger"
)

// Server is the transport that the application layer drives. The HTTP server
// implements it, and tests substitute a fake to verify shutdown behaviour
// without touching the operating system.
type Server interface {
	Start(ctx context.Context) error
	Shutdown(ctx context.Context) error
}

// SignalContext derives a context that is cancelled on SIGINT or SIGTERM.
type SignalContext func(parent context.Context) (context.Context, context.CancelFunc)

// NotifyContext is the production SignalContext implementation.
func NotifyContext(parent context.Context) (context.Context, context.CancelFunc) {
	return signal.NotifyContext(parent, syscall.SIGINT, syscall.SIGTERM)
}

// App owns the application lifecycle and its dependencies. Nothing is stored
// in package level state, so several instances can coexist in one process and
// tests stay isolated.
type App struct {
	config        config.Config
	logger        *slog.Logger
	service       domain.Service
	closers       []func(context.Context) error
	signalContext SignalContext
}

// Option customises an App.
type Option func(*App)

// WithCloser registers a resource released during shutdown, in registration
// order, using the shutdown context.
func WithCloser(closer func(context.Context) error) Option {
	return func(a *App) {
		if closer != nil {
			a.closers = append(a.closers, closer)
		}
	}
}

// WithSignalContext replaces the signal handling strategy. Tests use it to
// cancel deterministically instead of raising real signals.
func WithSignalContext(fn SignalContext) Option {
	return func(a *App) {
		if fn != nil {
			a.signalContext = fn
		}
	}
}

// New wires the application. The service is required so that every transport
// shares the same business logic.
func New(cfg config.Config, log *slog.Logger, svc domain.Service, opts ...Option) *App {
	if log == nil {
		log = logger.Noop()
	}
	a := &App{
		config:        cfg,
		logger:        log,
		service:       svc,
		signalContext: NotifyContext,
	}
	for _, opt := range opts {
		if opt != nil {
			opt(a)
		}
	}
	return a
}

// Config exposes the resolved configuration.
func (a *App) Config() config.Config { return a.config }

// Service exposes the shared application service.
func (a *App) Service() domain.Service { return a.service }

// Run executes the core application: it performs one unit of work through the
// shared service and then stays alive until the context is cancelled.
func (a *App) Run(ctx context.Context) error {
	if a.service == nil {
		return errors.New("application service is not configured")
	}

	ctx, stop := a.signalContext(ctx)
	defer stop()

	log := logger.FromContextOr(ctx, a.logger).With(slog.String("component", "app"))
	log.InfoContext(ctx, "application started")

	greeting, err := a.service.Greet(ctx, a.config.AppName)
	if err != nil {
		if ctx.Err() != nil {
			// Cancellation is a requested stop, not a failure, so the process can
			// still exit with a success status.
			log.InfoContext(ctx, "application stopped before the work completed")
			return a.closeResources(ctx, log)
		}
		return fmt.Errorf("run application: %w", err)
	}
	log.InfoContext(ctx, "application work completed", slog.String("greeting", greeting.Message))

	<-ctx.Done()
	log.InfoContext(ctx, "application stopped")
	return a.closeResources(ctx, log)
}

// Serve starts the transport and blocks until the context is cancelled or the
// transport stops on its own. On cancellation the server is drained within the
// configured shutdown timeout.
func (a *App) Serve(ctx context.Context, srv Server) error {
	if srv == nil {
		return errors.New("server is not configured")
	}

	ctx, stop := a.signalContext(ctx)
	defer stop()

	log := logger.FromContextOr(ctx, a.logger).With(slog.String("component", "app"))
	log.InfoContext(ctx, "server lifecycle started")

	// The buffered channel keeps the Start goroutine from blocking on send, so
	// it always terminates and the process stays free of goroutine leaks.
	errCh := make(chan error, 1)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		errCh <- srv.Start(ctx)
	}()

	released := false
	release := func() error {
		if released {
			return nil
		}
		released = true
		return a.closeResources(ctx, log)
	}

	var lifecycleErr error
	drained := false
	select {
	case err := <-errCh:
		drained = true
		if err != nil {
			log.ErrorContext(ctx, "server stopped unexpectedly", slog.String("error", err.Error()))
			lifecycleErr = fmt.Errorf("server stopped unexpectedly: %w", err)
		}
	case <-ctx.Done():
		lifecycleErr = release()
	}

	if drained {
		wg.Wait()
	} else {
		lifecycleErr = errors.Join(lifecycleErr, a.waitForStop(ctx, errCh, &wg, log))
	}

	// Resources are released even when the transport failed to start, so a
	// partially initialised application does not leak anything.
	return errors.Join(lifecycleErr, release())
}

// waitForStop collects the error reported by Start after a drain request,
// bounded by the shutdown timeout so a broken transport cannot block exit.
func (a *App) waitForStop(ctx context.Context, errCh <-chan error, wg *sync.WaitGroup, log *slog.Logger) error {
	timer := time.NewTimer(a.config.HTTP.ShutdownTimeout)
	defer timer.Stop()

	select {
	case err := <-errCh:
		wg.Wait()
		if err != nil && !isExpectedStop(err) {
			return fmt.Errorf("server returned an error during shutdown: %w", err)
		}
		log.InfoContext(ctx, "server stopped")
		return nil
	case <-timer.C:
		return fmt.Errorf("timed out after %s waiting for the server to stop", a.config.HTTP.ShutdownTimeout)
	}
}

// closeResources releases every registered resource using a context that keeps
// the parent values but is no longer cancelled, bounded by the shutdown
// timeout.
func (a *App) closeResources(ctx context.Context, log *slog.Logger) error {
	timeout := a.config.HTTP.ShutdownTimeout
	if timeout <= 0 {
		timeout = config.DefaultShutdownTimeout
	}

	shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), timeout)
	defer cancel()

	var errs []error
	for _, closer := range a.closers {
		if err := closer(shutdownCtx); err != nil {
			errs = append(errs, fmt.Errorf("close resource: %w", err))
		}
	}

	if len(errs) == 0 {
		log.InfoContext(shutdownCtx, "resources released")
		return nil
	}
	joined := errors.Join(errs...)
	log.ErrorContext(shutdownCtx, "resource release failed", slog.String("error", joined.Error()))
	return joined
}

func isExpectedStop(err error) bool {
	return errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)
}
