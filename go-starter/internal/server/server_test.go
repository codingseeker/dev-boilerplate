package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/codingseeker/go-starter/internal/config"
	"github.com/codingseeker/go-starter/internal/domain"
	"github.com/codingseeker/go-starter/internal/logger"
	"github.com/codingseeker/go-starter/internal/service"
)

type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
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
	cfg.AppName = "server-test"
	cfg.Environment = config.EnvironmentTesting
	cfg.LogLevel = config.LogLevelDebug
	return cfg
}

func newTestServer(t *testing.T, svc domain.Service) (*Server, *httptest.Server, *lockedBuffer) {
	t.Helper()

	cfg := testConfig()
	logs := &lockedBuffer{}
	log, err := logger.New(cfg, logs)
	if err != nil {
		t.Fatalf("logger.New() error = %v", err)
	}

	srv := New(cfg, log, svc)
	if srv == nil {
		t.Fatal("New() = nil")
	}

	httpTest := httptest.NewServer(srv.Handler())
	t.Cleanup(httpTest.Close)
	return srv, httpTest, logs
}

func newTestService() *service.ApplicationService {
	return service.New(testConfig())
}

func do(t *testing.T, ts *httptest.Server, method, path string, body io.Reader, headers map[string]string) *http.Response {
	t.Helper()

	req, err := http.NewRequestWithContext(context.Background(), method, ts.URL+path, body)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	for key, value := range headers {
		req.Header.Set(key, value)
	}

	resp, err := ts.Client().Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })
	return resp
}

func decodeJSON(t *testing.T, resp *http.Response) map[string]any {
	t.Helper()
	var payload map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	return payload
}

func TestHealth(t *testing.T) {
	_, ts, _ := newTestServer(t, newTestService())

	resp := do(t, ts, http.MethodGet, "/health", nil, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /health status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("Content-Type = %q, want JSON", ct)
	}

	payload := decodeJSON(t, resp)
	if payload["status"] != domain.StatusOK {
		t.Errorf("status = %v, want %q", payload["status"], domain.StatusOK)
	}
	if payload["app"] != "server-test" {
		t.Errorf("app = %v, want server-test", payload["app"])
	}
	if payload["version"] == "" || payload["version"] == nil {
		t.Error("version must be reported")
	}
	if payload["checks"] == nil {
		t.Error("checks must be reported")
	}
}

func TestReady(t *testing.T) {
	svc := newTestService()
	_, ts, _ := newTestServer(t, svc)

	resp := do(t, ts, http.MethodGet, "/ready", nil, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /ready status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
	if payload := decodeJSON(t, resp); payload["status"] != domain.StatusOK {
		t.Errorf("status = %v, want %q", payload["status"], domain.StatusOK)
	}

	if err := svc.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	resp = do(t, ts, http.MethodGet, "/ready", nil, nil)
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("GET /ready while draining status = %d, want %d", resp.StatusCode, http.StatusServiceUnavailable)
	}
	payload := decodeJSON(t, resp)
	if payload["status"] != domain.StatusDegraded {
		t.Errorf("status = %v, want %q", payload["status"], domain.StatusDegraded)
	}
}

func TestHealthWithoutService(t *testing.T) {
	_, ts, _ := newTestServer(t, nil)

	for _, path := range []string{"/health", "/ready"} {
		resp := do(t, ts, http.MethodGet, path, nil, nil)
		if resp.StatusCode != http.StatusServiceUnavailable {
			t.Errorf("GET %s status = %d, want %d", path, resp.StatusCode, http.StatusServiceUnavailable)
		}
	}
}

func TestGreet(t *testing.T) {
	_, ts, _ := newTestServer(t, newTestService())

	resp := do(t, ts, http.MethodGet, "/v1/greet?name=ada", nil, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /v1/greet status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
	payload := decodeJSON(t, resp)
	if payload["name"] != "ada" {
		t.Errorf("name = %v, want ada", payload["name"])
	}
	if message, _ := payload["message"].(string); !strings.Contains(message, "server-test") {
		t.Errorf("message = %v, want it to mention the application name", payload["message"])
	}
}

func TestGreetValidation(t *testing.T) {
	_, ts, _ := newTestServer(t, newTestService())

	tests := []struct {
		name string
		path string
	}{
		{"missing_name", "/v1/greet"},
		{"blank_name", "/v1/greet?name=%20%20"},
		{"name_too_long", "/v1/greet?name=" + strings.Repeat("a", domain.MaxNameLength+1)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp := do(t, ts, http.MethodGet, tt.path, nil, nil)
			if resp.StatusCode != http.StatusBadRequest {
				t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusBadRequest)
			}
			apiErr := decodeError(t, resp)
			if apiErr.Code != codeInvalidRequest {
				t.Errorf("code = %q, want %q", apiErr.Code, codeInvalidRequest)
			}
			if apiErr.RequestID == "" {
				t.Error("error responses must carry the request id")
			}
		})
	}
}

func TestEcho(t *testing.T) {
	_, ts, _ := newTestServer(t, newTestService())

	resp := do(t, ts, http.MethodPost, "/v1/echo", strings.NewReader(`{"message":"hello"}`), nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("POST /v1/echo status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
	if payload := decodeJSON(t, resp); payload["message"] != "hello" {
		t.Errorf("message = %v, want hello", payload["message"])
	}
}

func TestEchoRejectsInvalidBodies(t *testing.T) {
	_, ts, _ := newTestServer(t, newTestService())

	tests := []struct {
		name        string
		body        string
		contentType string
		wantStatus  int
		wantCode    string
	}{
		{"empty_body", "", "application/json", http.StatusBadRequest, codeInvalidJSON},
		{"blank_body", "   \n", "application/json", http.StatusBadRequest, codeInvalidJSON},
		{"broken_json", `{"message":`, "application/json", http.StatusBadRequest, codeInvalidJSON},
		{"not_an_object", `"hello"`, "application/json", http.StatusBadRequest, codeInvalidJSON},
		{"empty_message", `{"message":"  "}`, "application/json", http.StatusBadRequest, codeInvalidRequest},
		{"wrong_content_type", `{"message":"hello"}`, "text/plain", http.StatusUnsupportedMediaType, codeUnsupported},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, ts.URL+"/v1/echo", strings.NewReader(tt.body))
			if err != nil {
				t.Fatalf("build request: %v", err)
			}
			req.Header.Set("Content-Type", tt.contentType)

			resp, err := ts.Client().Do(req)
			if err != nil {
				t.Fatalf("POST /v1/echo: %v", err)
			}
			defer func() { _ = resp.Body.Close() }()

			if resp.StatusCode != tt.wantStatus {
				t.Fatalf("status = %d, want %d", resp.StatusCode, tt.wantStatus)
			}
			apiErr := decodeError(t, resp)
			if apiErr.Code != tt.wantCode {
				t.Errorf("code = %q, want %q", apiErr.Code, tt.wantCode)
			}
		})
	}
}

func TestRequestBodyLimit(t *testing.T) {
	_, ts, _ := newTestServer(t, newTestService())

	oversized := `{"message":"` + strings.Repeat("a", maxRequestBodySize) + `"}`
	resp := do(t, ts, http.MethodPost, "/v1/echo", strings.NewReader(oversized), nil)
	if resp.StatusCode != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusRequestEntityTooLarge)
	}
	if apiErr := decodeError(t, resp); apiErr.Code != codeBodyTooLarge {
		t.Errorf("code = %q, want %q", apiErr.Code, codeBodyTooLarge)
	}
}

func TestBodyAtTheLimitIsAccepted(t *testing.T) {
	_, ts, _ := newTestServer(t, newTestService())

	// A body just below the limit must still be processed, proving the limit
	// is a limit and not a blanket rejection.
	prefix := `{"message":"`
	suffix := `"}`
	filler := strings.Repeat("a", maxRequestBodySize-len(prefix)-len(suffix)-1)
	body := prefix + filler + suffix

	resp := do(t, ts, http.MethodPost, "/v1/echo", strings.NewReader(body), nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
}

func TestMethodNotAllowed(t *testing.T) {
	_, ts, _ := newTestServer(t, newTestService())

	tests := []struct {
		method string
		path   string
		allow  string
	}{
		{http.MethodPost, "/health", "GET"},
		{http.MethodDelete, "/v1/greet", "GET"},
		{http.MethodGet, "/v1/echo", "POST"},
	}

	for _, tt := range tests {
		t.Run(tt.method+" "+tt.path, func(t *testing.T) {
			resp := do(t, ts, tt.method, tt.path, nil, nil)
			if resp.StatusCode != http.StatusMethodNotAllowed {
				t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusMethodNotAllowed)
			}
			if allow := resp.Header.Get("Allow"); !strings.Contains(allow, tt.allow) {
				t.Errorf("Allow = %q, want it to contain %q", allow, tt.allow)
			}
			if apiErr := decodeError(t, resp); apiErr.Code != codeMethodNotAllow {
				t.Errorf("code = %q, want %q", apiErr.Code, codeMethodNotAllow)
			}
		})
	}
}

func TestUnknownPath(t *testing.T) {
	_, ts, _ := newTestServer(t, newTestService())

	resp := do(t, ts, http.MethodGet, "/does-not-exist", nil, nil)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusNotFound)
	}
	if apiErr := decodeError(t, resp); apiErr.Code != codeNotFound {
		t.Errorf("code = %q, want %q", apiErr.Code, codeNotFound)
	}
}

func TestSecurityHeaders(t *testing.T) {
	_, ts, _ := newTestServer(t, newTestService())

	resp := do(t, ts, http.MethodGet, "/health", nil, nil)
	want := map[string]string{
		"X-Content-Type-Options": "nosniff",
		"X-Frame-Options":        "DENY",
		"Referrer-Policy":        "no-referrer",
		"Cache-Control":          "no-store",
	}
	for header, value := range want {
		if got := resp.Header.Get(header); got != value {
			t.Errorf("%s = %q, want %q", header, got, value)
		}
	}
}

func TestRequestIDIsPropagated(t *testing.T) {
	_, ts, _ := newTestServer(t, newTestService())

	t.Run("generated", func(t *testing.T) {
		resp := do(t, ts, http.MethodGet, "/health", nil, nil)
		if resp.Header.Get(requestIDHeader) == "" {
			t.Error("a request id must always be returned")
		}
	})

	t.Run("propagated", func(t *testing.T) {
		resp := do(t, ts, http.MethodGet, "/health", nil, map[string]string{requestIDHeader: "caller-supplied-id"})
		if got := resp.Header.Get(requestIDHeader); got != "caller-supplied-id" {
			t.Errorf("%s = %q, want the caller supplied id", requestIDHeader, got)
		}
	})
}

func TestLogsContainNoRequestContent(t *testing.T) {
	_, ts, logs := newTestServer(t, newTestService())

	const secret = "super-secret-payload"
	body := `{"message":"` + secret + `"}`
	if resp := do(t, ts, http.MethodPost, "/v1/echo", strings.NewReader(body), nil); resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
	if strings.Contains(logs.String(), secret) {
		t.Errorf("request content leaked into the logs:\n%s", logs.String())
	}

	// The access log still records the correlation data an operator needs.
	out := logs.String()
	for _, want := range []string{"http request", "status=", "duration=", "request_id="} {
		if !strings.Contains(out, want) {
			t.Errorf("access log does not contain %q:\n%s", want, out)
		}
	}
}

func TestInternalErrorsAreNotLeaked(t *testing.T) {
	_, ts, logs := newTestServer(t, erroringService{cause: errors.New("connection to 10.0.0.5:5432 refused")})

	resp := do(t, ts, http.MethodGet, "/v1/greet?name=ada", nil, nil)
	if resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusInternalServerError)
	}
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if strings.Contains(string(raw), "5432") {
		t.Errorf("internal error leaked to the client: %s", raw)
	}

	var payload errorResponse
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatalf("decode error response %q: %v", raw, err)
	}
	if payload.Error.Code != codeInternal || payload.Error.Message != messageInternal {
		t.Errorf("error = %+v, want a generic internal error", payload.Error)
	}
	if !strings.Contains(logs.String(), "5432") {
		t.Errorf("the internal cause must be logged:\n%s", logs.String())
	}
}

func TestPanicIsRecovered(t *testing.T) {
	_, ts, logs := newTestServer(t, &panickingService{})

	resp := do(t, ts, http.MethodGet, "/health", nil, nil)
	if resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusInternalServerError)
	}
	if apiErr := decodeError(t, resp); apiErr.Code != codeInternal {
		t.Errorf("code = %q, want %q", apiErr.Code, codeInternal)
	}
	if !strings.Contains(logs.String(), "panic while handling request") {
		t.Errorf("the panic must be logged:\n%s", logs.String())
	}

	// The server keeps serving after a panic.
	if resp := do(t, ts, http.MethodGet, "/health", nil, nil); resp.StatusCode == http.StatusInternalServerError {
		t.Error("a panicking request must not poison later requests")
	}
}

func TestStartAndShutdown(t *testing.T) {
	cfg := testConfig()
	cfg.HTTP.Host = "127.0.0.1"
	cfg.HTTP.Port = 0

	logs := &lockedBuffer{}
	log, err := logger.New(cfg, logs)
	if err != nil {
		t.Fatalf("logger.New() error = %v", err)
	}

	srv := New(cfg, log, newTestService())
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan error, 1)
	go func() { done <- srv.Start(ctx) }()

	addr := waitForAddr(t, srv)
	if !strings.HasPrefix(addr, "127.0.0.1:") {
		t.Errorf("Addr() = %q, want the loopback address", addr)
	}

	resp, err := httpGet(t, "http://"+addr+"/health")
	if err != nil {
		t.Fatalf("GET /health over the bound socket: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("GET /health status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		t.Fatalf("Shutdown() error = %v", err)
	}

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Start() error = %v, want nil after a graceful shutdown", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Start() did not return after Shutdown()")
	}

	if _, err := httpGet(t, "http://"+addr+"/health"); err == nil {
		t.Error("the listener must be closed after Shutdown()")
	}
}

func TestStartFailsOnUnusableAddress(t *testing.T) {
	cfg := testConfig()
	cfg.HTTP.Host = "203.0.113.1" // reserved, never routable
	cfg.HTTP.Port = 1

	srv := New(cfg, logger.Noop(), newTestService())
	err := srv.Start(context.Background())
	if err == nil {
		t.Fatal("Start() error = nil, want a bind failure")
	}
	if !strings.Contains(err.Error(), "listen") {
		t.Errorf("Start() error = %v, want it to mention the failed bind", err)
	}
}

func TestStartTwiceIsRejected(t *testing.T) {
	cfg := testConfig()
	cfg.HTTP.Host = "127.0.0.1"
	cfg.HTTP.Port = 0

	srv := New(cfg, logger.Noop(), newTestService())
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan error, 1)
	go func() { done <- srv.Start(ctx) }()
	waitForAddr(t, srv)

	if err := srv.Start(context.Background()); err == nil {
		t.Fatal("Start() error = nil, want a rejection of the second start")
	}

	if err := srv.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown() error = %v", err)
	}
	<-done
}

func TestShutdownBeforeStartIsSafe(t *testing.T) {
	srv := New(testConfig(), logger.Noop(), newTestService())

	if err := srv.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown() error = %v, want nil", err)
	}
	if err := srv.Shutdown(context.Background()); err != nil {
		t.Fatalf("second Shutdown() error = %v, want nil", err)
	}
}

func TestShutdownForcesCloseWhenDrainDeadlineExpires(t *testing.T) {
	cfg := testConfig()
	cfg.HTTP.Host = "127.0.0.1"
	cfg.HTTP.Port = 0
	cfg.HTTP.ShutdownTimeout = 50 * time.Millisecond

	srv := New(cfg, logger.Noop(), newTestService())

	// Occupy the server with a request that never completes on its own.
	var enteredOnce, releaseOnce sync.Once
	entered := make(chan struct{})
	release := make(chan struct{})
	releaseAll := func() { releaseOnce.Do(func() { close(release) }) }
	defer releaseAll()

	srv.handler = http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		enteredOnce.Do(func() { close(entered) })
		<-release
		w.WriteHeader(http.StatusOK)
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- srv.Start(ctx) }()
	addr := waitForAddr(t, srv)

	requestDone := make(chan struct{})
	go func() {
		defer close(requestDone)
		resp, err := httpGet(t, "http://"+addr+"/health")
		if err == nil {
			_ = resp.Body.Close()
		}
	}()
	<-entered

	err := srv.Shutdown(context.Background())
	if err == nil {
		t.Fatal("Shutdown() error = nil, want the expired drain deadline")
	}
	if !strings.Contains(err.Error(), "graceful shutdown failed") {
		t.Errorf("Shutdown() error = %v, want it to report the failed drain", err)
	}

	<-requestDone
	releaseAll()

	if _, err := httpGet(t, "http://"+addr+"/health"); err == nil {
		t.Error("the listener must be closed even when the drain deadline expires")
	}
	if err := <-done; err != nil {
		t.Errorf("Start() error = %v, want nil", err)
	}
}

func TestNewHTTPServerAppliesSafeDefaults(t *testing.T) {
	cfg := testConfig()
	cfg.HTTP.ReadTimeout = 7 * time.Second
	cfg.HTTP.WriteTimeout = 8 * time.Second
	cfg.HTTP.IdleTimeout = 9 * time.Second

	srv := New(cfg, logger.Noop(), newTestService())
	httpSrv := srv.newHTTPServer()

	if httpSrv.Addr != cfg.HTTP.Addr() {
		t.Errorf("Addr = %q, want %q", httpSrv.Addr, cfg.HTTP.Addr())
	}
	if httpSrv.ReadHeaderTimeout != cfg.HTTP.ReadTimeout {
		t.Errorf("ReadHeaderTimeout = %s, want %s", httpSrv.ReadHeaderTimeout, cfg.HTTP.ReadTimeout)
	}
	if httpSrv.ReadTimeout != cfg.HTTP.ReadTimeout || httpSrv.WriteTimeout != cfg.HTTP.WriteTimeout {
		t.Errorf("timeouts = %s/%s, want %s/%s",
			httpSrv.ReadTimeout, httpSrv.WriteTimeout, cfg.HTTP.ReadTimeout, cfg.HTTP.WriteTimeout)
	}
	if httpSrv.IdleTimeout != cfg.HTTP.IdleTimeout {
		t.Errorf("IdleTimeout = %s, want %s", httpSrv.IdleTimeout, cfg.HTTP.IdleTimeout)
	}
	if httpSrv.MaxHeaderBytes != maxHeaderBytes {
		t.Errorf("MaxHeaderBytes = %d, want %d", httpSrv.MaxHeaderBytes, maxHeaderBytes)
	}
	if httpSrv.ErrorLog == nil {
		t.Error("ErrorLog must forward stdlib server errors to the structured logger")
	}
	if httpSrv.Handler == nil {
		t.Error("Handler must be the decorated router")
	}
}

func TestFirstIP(t *testing.T) {
	tests := []struct {
		header string
		want   string
	}{
		{"1.2.3.4", "1.2.3.4"},
		{"1.2.3.4, 5.6.7.8", "1.2.3.4"},
		{"  1.2.3.4 , 5.6.7.8  ", "1.2.3.4"},
	}
	for _, tt := range tests {
		if got := firstIP(tt.header); got != tt.want {
			t.Errorf("firstIP(%q) = %q, want %q", tt.header, got, tt.want)
		}
	}
}

func TestIsBodyTooLarge(t *testing.T) {
	var tooLarge *http.MaxBytesError
	err := &http.MaxBytesError{Limit: maxRequestBodySize}
	if !isBodyTooLarge(err) {
		t.Error("MaxBytesError must be detected")
	}
	if !errors.As(error(err), &tooLarge) {
		t.Error("errors.As must recognise MaxBytesError")
	}
	if isBodyTooLarge(io.ErrUnexpectedEOF) {
		t.Error("unrelated errors must not be treated as oversized bodies")
	}
}

// Helpers.

func decodeError(t *testing.T, resp *http.Response) apiError {
	t.Helper()
	var payload errorResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	if payload.Error.Code == "" {
		t.Fatalf("error response without a code: %+v", payload)
	}
	return payload.Error
}

// httpGet performs a GET with an explicit context, which noctx requires and
// which keeps the tests honest about context usage.
func httpGet(t *testing.T, url string) (*http.Response, error) {
	t.Helper()

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	return http.DefaultClient.Do(req)
}

func waitForAddr(t *testing.T, srv *Server) string {
	t.Helper()
	var addr string
	waitFor(t, func() bool {
		addr = srv.Addr()
		return addr != ""
	})
	return addr
}

func waitFor(t *testing.T, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !condition() {
		if time.Now().After(deadline) {
			t.Fatal("condition was not met in time")
		}
		time.Sleep(time.Millisecond)
	}
}

type erroringService struct{ cause error }

func (s erroringService) Health(context.Context) (domain.HealthStatus, error) {
	return domain.HealthStatus{}, s.cause
}

func (s erroringService) Ready(context.Context) (domain.HealthStatus, error) {
	return domain.HealthStatus{}, s.cause
}

func (s erroringService) Greet(context.Context, string) (domain.Greeting, error) {
	return domain.Greeting{}, s.cause
}

// panickingService explodes on the first liveness check only, so the test can
// prove that later requests are still served.
type panickingService struct {
	once sync.Once
}

func (s *panickingService) Health(context.Context) (domain.HealthStatus, error) {
	s.once.Do(func() { panic("handler exploded") })
	return domain.HealthStatus{Status: domain.StatusOK}, nil
}

func (s *panickingService) Ready(ctx context.Context) (domain.HealthStatus, error) {
	return s.Health(ctx)
}

func (s *panickingService) Greet(context.Context, string) (domain.Greeting, error) {
	return domain.Greeting{}, errors.New("not implemented")
}
