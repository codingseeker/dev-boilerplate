// Package tests holds black box integration tests. They exercise the built
// binary and the wired application, so they catch problems that unit tests
// inside a package cannot see.
package tests

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/codingseeker/go-starter/internal/app"
	"github.com/codingseeker/go-starter/internal/buildinfo"
	"github.com/codingseeker/go-starter/internal/cli"
	"github.com/codingseeker/go-starter/internal/config"
	"github.com/codingseeker/go-starter/internal/logger"
	"github.com/codingseeker/go-starter/internal/server"
	"github.com/codingseeker/go-starter/internal/service"
)

func TestMain(m *testing.M) {
	os.Exit(m.Run())
}

func TestServeServesTheAPIAndShutsDownGracefully(t *testing.T) {
	cfg := config.Default()
	cfg.AppName = "integration"
	cfg.Environment = config.EnvironmentTesting
	cfg.LogLevel = config.LogLevelDebug
	cfg.HTTP.Host = "127.0.0.1"
	cfg.HTTP.Port = 0
	cfg.HTTP.ShutdownTimeout = 2 * time.Second

	logs := &syncBuffer{}
	log, err := logger.New(cfg, logs)
	if err != nil {
		t.Fatalf("logger.New() error = %v", err)
	}

	svc := service.New(cfg)
	httpServer := server.New(cfg, log, svc)
	application := app.New(cfg, log, svc,
		app.WithSignalContext(func(parent context.Context) (context.Context, context.CancelFunc) {
			return context.WithCancel(parent)
		}),
		app.WithCloser(func(context.Context) error { return svc.Close() }),
		app.WithCloser(httpServer.Shutdown),
	)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- application.Serve(ctx, httpServer) }()

	base := waitForAddr(t, httpServer)

	t.Run("health", func(t *testing.T) {
		body, status := get(t, base+"/health")
		if status != http.StatusOK {
			t.Fatalf("GET /health status = %d, want %d (body: %s)", status, http.StatusOK, body)
		}
		if !strings.Contains(body, `"status":"ok"`) || !strings.Contains(body, `"app":"integration"`) {
			t.Errorf("GET /health body = %s, want the resolved application identity", body)
		}
	})

	t.Run("ready", func(t *testing.T) {
		body, status := get(t, base+"/ready")
		if status != http.StatusOK {
			t.Fatalf("GET /ready status = %d, want %d (body: %s)", status, http.StatusOK, body)
		}
	})

	t.Run("greet_uses_the_shared_service", func(t *testing.T) {
		body, status := get(t, base+"/v1/greet?name=ada")
		if status != http.StatusOK {
			t.Fatalf("GET /v1/greet status = %d, want %d (body: %s)", status, http.StatusOK, body)
		}
		var payload struct {
			Name    string `json:"name"`
			Message string `json:"message"`
		}
		if err := json.Unmarshal([]byte(body), &payload); err != nil {
			t.Fatalf("decode greet response %q: %v", body, err)
		}
		if payload.Name != "ada" {
			t.Errorf("name = %q, want ada", payload.Name)
		}
		if !strings.Contains(payload.Message, "integration") {
			t.Errorf("message = %q, want it to mention the application name", payload.Message)
		}
	})

	t.Run("invalid_request", func(t *testing.T) {
		body, status := get(t, base+"/v1/greet")
		if status != http.StatusBadRequest {
			t.Fatalf("status = %d, want %d (body: %s)", status, http.StatusBadRequest, body)
		}
		if !strings.Contains(body, `"code":"invalid_request"`) {
			t.Errorf("body = %s, want the documented error shape", body)
		}
	})

	t.Run("request_id_is_returned", func(t *testing.T) {
		req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, base+"/health", nil)
		if err != nil {
			t.Fatalf("build request: %v", err)
		}
		req.Header.Set("X-Request-Id", "integration-trace")

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("GET /health: %v", err)
		}
		defer func() { _ = resp.Body.Close() }()

		if got := resp.Header.Get("X-Request-Id"); got != "integration-trace" {
			t.Errorf("X-Request-Id = %q, want the propagated value", got)
		}
	})

	t.Run("graceful_shutdown", func(t *testing.T) {
		cancel()

		select {
		case err := <-done:
			if err != nil {
				t.Fatalf("Serve() error = %v, want nil", err)
			}
		case <-time.After(10 * time.Second):
			t.Fatal("Serve() did not return after cancellation")
		}

		if _, err := httpGet(t, base+"/health"); err == nil {
			t.Error("the listener must be closed after the shutdown")
		}

		out := logs.String()
		for _, want := range []string{"http server draining", "http server stopped", "resources released"} {
			if !strings.Contains(out, want) {
				t.Errorf("shutdown log does not contain %q:\n%s", want, out)
			}
		}
	})
}

func TestBinaryEndToEnd(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping binary test in short mode")
	}
	goTool, err := exec.LookPath("go")
	if err != nil {
		t.Skip("go toolchain not available, skipping the binary test")
	}

	dir := t.TempDir()
	binary := filepath.Join(dir, "app")
	buildCtx, buildCancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer buildCancel()

	build := exec.CommandContext(buildCtx, goTool, "build",
		"-ldflags", "-X github.com/codingseeker/go-starter/internal/buildinfo.Version=9.9.9-test",
		"-o", binary, "./cmd/app")
	build.Dir = repoRoot(t)
	build.Env = append(os.Environ(), "GOCACHE="+filepath.Join(dir, "cache"))
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}

	t.Run("version_uses_the_injected_version", func(t *testing.T) {
		out, code := runBinary(t, binary, dir, nil, "version")
		if code != cli.ExitOK {
			t.Fatalf("exit code = %d, want %d (output: %s)", code, cli.ExitOK, out)
		}
		if !strings.Contains(out, "9.9.9-test") {
			t.Errorf("output = %q, want the injected version", out)
		}
	})

	t.Run("help", func(t *testing.T) {
		out, code := runBinary(t, binary, dir, nil, "--help")
		if code != cli.ExitOK {
			t.Fatalf("exit code = %d, want %d (output: %s)", code, cli.ExitOK, out)
		}
		for _, want := range []string{"Usage:", "run", "serve", "version"} {
			if !strings.Contains(out, want) {
				t.Errorf("help output does not mention %q:\n%s", want, out)
			}
		}
	})

	t.Run("unknown_command_exits_with_usage_code", func(t *testing.T) {
		out, code := runBinary(t, binary, dir, nil, "deploy")
		if code != cli.ExitUsage {
			t.Fatalf("exit code = %d, want %d (output: %s)", code, cli.ExitUsage, out)
		}
		if !strings.Contains(out, "unknown command") {
			t.Errorf("output = %q, want a clear error", out)
		}
	})

	t.Run("invalid_configuration_exits_with_usage_code", func(t *testing.T) {
		env := map[string]string{config.EnvEnvironment: "staging"}
		out, code := runBinary(t, binary, dir, env, "serve")
		if code != cli.ExitUsage {
			t.Fatalf("exit code = %d, want %d (output: %s)", code, cli.ExitUsage, out)
		}
		if !strings.Contains(out, config.EnvEnvironment) {
			t.Errorf("output = %q, want it to name the offending variable", out)
		}
	})

	t.Run("invalid_port_exits_with_usage_code", func(t *testing.T) {
		env := map[string]string{config.EnvHTTPPort: "not-a-port"}
		out, code := runBinary(t, binary, dir, env, "serve")
		if code != cli.ExitUsage {
			t.Fatalf("exit code = %d, want %d (output: %s)", code, cli.ExitUsage, out)
		}
		if !strings.Contains(out, config.EnvHTTPPort) {
			t.Errorf("output = %q, want it to name the offending variable", out)
		}
	})

	for _, signal := range []os.Signal{os.Interrupt, syscall.SIGTERM} {
		t.Run("serve_stops_on_"+strings.TrimPrefix(signal.String(), "signal "), func(t *testing.T) {
			port := freePort(t)
			env := map[string]string{
				config.EnvAppName:     "binary",
				config.EnvEnvironment: config.EnvironmentTesting,
				config.EnvLogLevel:    config.LogLevelInfo,
				config.EnvHTTPHost:    "127.0.0.1",
				config.EnvHTTPPort:    fmt.Sprintf("%d", port),
			}

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			cmd := exec.CommandContext(ctx, binary, "serve")
			cmd.Dir = dir
			cmd.Env = mergedEnv(env)
			// A .env file in the working directory must not leak into the run.
			if err := os.WriteFile(filepath.Join(dir, ".env"), []byte(config.EnvAppName+"=leaked\n"), 0o600); err != nil {
				t.Fatalf("write .env: %v", err)
			}

			var out bytes.Buffer
			cmd.Stdout = &out
			cmd.Stderr = &out

			if err := cmd.Start(); err != nil {
				t.Fatalf("start binary: %v", err)
			}

			base := fmt.Sprintf("http://127.0.0.1:%d", port)
			waitForHealth(t, base)

			body, status := get(t, base+"/health")
			if status != http.StatusOK {
				t.Fatalf("GET /health status = %d, want %d (body: %s)", status, http.StatusOK, body)
			}
			if !strings.Contains(body, `"app":"binary"`) {
				t.Errorf("GET /health body = %s, want the environment value to win over .env", body)
			}

			if err := cmd.Process.Signal(signal); err != nil {
				t.Fatalf("signal binary: %v", err)
			}

			waitErr := make(chan error, 1)
			go func() { waitErr <- cmd.Wait() }()
			select {
			case err := <-waitErr:
				if err != nil {
					t.Fatalf("binary exited with %v, want a graceful exit (output: %s)", err, out.String())
				}
			case <-time.After(15 * time.Second):
				_ = cmd.Process.Kill()
				t.Fatalf("the binary did not exit after %v (output: %s)", signal, out.String())
			}

			if _, err := httpGet(t, base+"/health"); err == nil {
				t.Error("the listener must be closed after the signal")
			}
			for _, want := range []string{"http server stopped", "resources released"} {
				if !strings.Contains(out.String(), want) {
					t.Errorf("output does not contain %q:\n%s", want, out.String())
				}
			}
		})
	}
}

func TestCLIUsesInjectedEnvironmentOnly(t *testing.T) {
	// The CLI layer must not read the process environment when a lookup
	// function is injected, which is what keeps parallel tests deterministic.
	t.Chdir(t.TempDir())

	var out, errOut bytes.Buffer
	command := &cli.CLI{
		Out:     &out,
		Err:     &errOut,
		Args:    []string{"app", "run", "--env=testing", "--log-level=error"},
		Getenv:  func(string) string { return "" },
		Version: buildinfo.Version,
		SignalContext: func(parent context.Context) (context.Context, context.CancelFunc) {
			return context.WithTimeout(parent, 10*time.Millisecond)
		},
		LoadEnvFile: func(string) (bool, error) { return false, nil },
	}

	if err := command.Run(context.Background()); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
}

// Helpers.

type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func get(t *testing.T, url string) (string, int) {
	t.Helper()

	resp, err := httpGet(t, url)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read %s: %v", url, err)
	}
	return string(body), resp.StatusCode
}

func waitForAddr(t *testing.T, srv *server.Server) string {
	t.Helper()

	deadline := time.Now().Add(10 * time.Second)
	for {
		if addr := srv.Addr(); addr != "" {
			return "http://" + addr
		}
		if time.Now().After(deadline) {
			t.Fatal("the server never reported its listen address")
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func httpGet(t *testing.T, url string) (*http.Response, error) {
	t.Helper()

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	return http.DefaultClient.Do(req)
}

func waitForHealth(t *testing.T, base string) {
	t.Helper()

	deadline := time.Now().Add(15 * time.Second)
	var lastErr error
	for time.Now().Before(deadline) {
		resp, err := httpGet(t, base+"/health")
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return
			}
			lastErr = fmt.Errorf("status %d", resp.StatusCode)
		} else {
			lastErr = err
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("the server never became healthy: %v", lastErr)
}

func freePort(t *testing.T) int {
	t.Helper()

	listener, err := (&net.ListenConfig{}).Listen(context.Background(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve port: %v", err)
	}
	defer func() { _ = listener.Close() }()

	addr, ok := listener.Addr().(*net.TCPAddr)
	if !ok {
		t.Fatalf("unexpected listener address type %T", listener.Addr())
	}
	return addr.Port
}

func runBinary(t *testing.T, binary, dir string, env map[string]string, args ...string) (string, int) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, binary, args...)
	cmd.Dir = dir
	cmd.Env = mergedEnv(env)

	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out

	err := cmd.Run()
	if err == nil {
		return out.String(), cli.ExitOK
	}

	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("run %v: %v", args, err)
	}
	return out.String(), exitErr.ExitCode()
}

func mergedEnv(overrides map[string]string) []string {
	env := os.Environ()
	for key, value := range overrides {
		env = append(env, key+"="+value)
	}
	return env
}

func repoRoot(t *testing.T) string {
	t.Helper()

	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	return filepath.Dir(wd)
}
