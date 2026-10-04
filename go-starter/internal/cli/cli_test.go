package cli

import (
	"bytes"
	"context"
	"errors"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/codingseeker/go-starter/internal/config"
	"github.com/codingseeker/go-starter/internal/platform/dotenv"
)

// lockedBuffer collects concurrent command output, because the logger writes
// from the lifecycle goroutine while the test reads.
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

// harness builds a CLI that never touches the process environment and that
// cancels itself instead of waiting for an operating system signal.
type harness struct {
	cli   *CLI
	out   *lockedBuffer
	errs  *lockedBuffer
	env   map[string]string
	calls int
}

func newHarness(t *testing.T, args ...string) *harness {
	t.Helper()

	// An empty .env in the working directory must never influence the result.
	t.Chdir(t.TempDir())

	h := &harness{
		out:  &lockedBuffer{},
		errs: &lockedBuffer{},
		env:  map[string]string{},
	}
	h.cli = &CLI{
		Out:           h.out,
		Err:           h.errs,
		Args:          append([]string{programName}, args...),
		Getenv:        h.getenv,
		SignalContext: h.signals,
		LoadEnvFile:   h.loadEnvFile,
		Version:       "1.2.3",
	}
	return h
}

func (h *harness) getenv(key string) string { return h.env[key] }

// loadEnvFile resolves the real dotenv parser against the in-memory
// environment of the harness, so precedence is verified without touching the
// process environment.
func (h *harness) loadEnvFile(path string) (bool, error) {
	return dotenv.LoadInto(path,
		func(key string) (string, bool) {
			value, ok := h.env[key]
			return value, ok
		},
		func(key, value string) error {
			h.env[key] = value
			return nil
		},
	)
}

func (h *harness) signals(parent context.Context) (context.Context, context.CancelFunc) {
	h.calls++
	return context.WithTimeout(parent, 20*time.Millisecond)
}

// run executes the command under test with a fresh context.
func (h *harness) run() error {
	return h.cli.Run(context.Background())
}

func TestHelp(t *testing.T) {
	for _, args := range [][]string{
		{"help"},
		{"--help"},
		{"-h"},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			h := newHarness(t, args...)
			if err := h.run(); err != nil {
				t.Fatalf("Run() error = %v, want nil", err)
			}
			out := h.out.String()
			for _, want := range []string{"Usage:", programName, "run", "serve", "version", "precedence"} {
				if !strings.Contains(out, want) {
					t.Errorf("help output does not mention %q:\n%s", want, out)
				}
			}
			if h.errs.String() != "" {
				t.Errorf("help must not write to stderr, got %q", h.errs.String())
			}
		})
	}
}

func TestVersion(t *testing.T) {
	for _, args := range [][]string{{"version"}, {"--version"}, {"-v"}} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			h := newHarness(t, args...)
			if err := h.run(); err != nil {
				t.Fatalf("Run() error = %v, want nil", err)
			}
			if got, want := strings.TrimSpace(h.out.String()), programName+" 1.2.3"; got != want {
				t.Errorf("version output = %q, want %q", got, want)
			}
		})
	}
}

func TestVersionFallsBackToDev(t *testing.T) {
	h := newHarness(t, "version")
	h.cli.Version = "  "

	if err := h.run(); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if !strings.Contains(h.out.String(), "dev") {
		t.Errorf("version output = %q, want the dev placeholder", h.out.String())
	}
}

func TestCommandLineFlagPrecedence(t *testing.T) {
	tests := []struct {
		name string
		args []string
		env  map[string]string
		want string
	}{
		{
			name: "flag_beats_environment",
			args: []string{"run", "--env=production"},
			env:  map[string]string{config.EnvEnvironment: config.EnvironmentTesting},
			want: config.EnvironmentProduction,
		},
		{
			name: "environment_beats_default",
			args: []string{"run"},
			env:  map[string]string{config.EnvEnvironment: config.EnvironmentTesting},
			want: config.EnvironmentTesting,
		},
		{
			name: "default_when_nothing_is_set",
			args: []string{"run"},
			env:  nil,
			want: config.EnvironmentDevelopment,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newHarness(t, tt.args...)
			h.env = tt.env

			opts, _, err := h.cli.parseFlags(tt.args[0], tt.args[1:])
			if err != nil {
				t.Fatalf("parseFlags() error = %v", err)
			}
			cfg, err := h.cli.resolveConfig(opts)
			if err != nil {
				t.Fatalf("resolveConfig() error = %v", err)
			}
			if cfg.Environment != tt.want {
				t.Errorf("Environment = %q, want %q", cfg.Environment, tt.want)
			}
		})
	}
}

func TestResolveConfigFlagOverrides(t *testing.T) {
	h := newHarness(t)
	h.env = map[string]string{
		config.EnvAppName:         "from-env",
		config.EnvHTTPHost:        "10.0.0.1",
		config.EnvHTTPPort:        "8081",
		config.EnvReadTimeout:     "1s",
		config.EnvWriteTimeout:    "2s",
		config.EnvIdleTimeout:     "3s",
		config.EnvShutdownTimeout: "4s",
	}

	opts, _, err := h.cli.parseFlags("serve", []string{
		"--app-name=from-flag",
		"--http-host=127.0.0.1",
		"--http-port=9999",
		"--http-read-timeout=11s",
		"--http-write-timeout=12s",
		"--http-idle-timeout=13s",
		"--http-shutdown-timeout=14s",
		"--log-level=error",
	})
	if err != nil {
		t.Fatalf("parseFlags() error = %v", err)
	}

	cfg, err := h.cli.resolveConfig(opts)
	if err != nil {
		t.Fatalf("resolveConfig() error = %v", err)
	}

	if cfg.AppName != "from-flag" {
		t.Errorf("AppName = %q, want from-flag", cfg.AppName)
	}
	if cfg.HTTP.Addr() != "127.0.0.1:9999" {
		t.Errorf("Addr() = %q, want 127.0.0.1:9999", cfg.HTTP.Addr())
	}
	if cfg.LogLevel != config.LogLevelError {
		t.Errorf("LogLevel = %q, want error", cfg.LogLevel)
	}
	if cfg.HTTP.ReadTimeout != 11*time.Second || cfg.HTTP.WriteTimeout != 12*time.Second ||
		cfg.HTTP.IdleTimeout != 13*time.Second || cfg.HTTP.ShutdownTimeout != 14*time.Second {
		t.Errorf("timeouts = %s/%s/%s/%s, want 11s/12s/13s/14s",
			cfg.HTTP.ReadTimeout, cfg.HTTP.WriteTimeout, cfg.HTTP.IdleTimeout, cfg.HTTP.ShutdownTimeout)
	}
}

func TestResolveConfigInvalidValues(t *testing.T) {
	tests := []struct {
		name string
		args []string
		env  map[string]string
		want string
	}{
		{"bad_environment", []string{"run", "--env=staging"}, nil, config.EnvEnvironment},
		{"bad_log_level", []string{"run", "--log-level=verbose"}, nil, config.EnvLogLevel},
		{"bad_port", []string{"run", "--http-port=70000"}, nil, config.EnvHTTPPort},
		{"bad_duration", []string{"run", "--http-read-timeout=soon"}, nil, "duration"},
		{"zero_duration", []string{"run", "--http-shutdown-timeout=0s"}, nil, "duration"},
		{"bad_environment_from_env", []string{"run"}, map[string]string{config.EnvEnvironment: "qa"}, config.EnvEnvironment},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newHarness(t, tt.args...)
			h.env = tt.env

			opts, help, err := h.cli.parseFlags(tt.args[0], tt.args[1:])
			if err != nil {
				if !strings.Contains(err.Error(), tt.want) {
					t.Fatalf("parseFlags() error = %v, want it to mention %q", err, tt.want)
				}
				return
			}
			if help {
				t.Fatal("parseFlags() reported help unexpectedly")
			}

			_, err = h.cli.resolveConfig(opts)
			if err == nil {
				t.Fatalf("resolveConfig() error = nil, want it to mention %q", tt.want)
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Errorf("resolveConfig() error = %v, want it to mention %q", err, tt.want)
			}
			if ExitCode(err) != ExitUsage {
				t.Errorf("ExitCode(%v) = %d, want %d", err, ExitCode(err), ExitUsage)
			}
		})
	}
}

func TestRunCommand(t *testing.T) {
	h := newHarness(t, "run", "--env=testing", "--log-level=info")

	if err := h.run(); err != nil {
		t.Fatalf("Run() error = %v, want nil", err)
	}
	if h.calls == 0 {
		t.Error("the run command must build an application with the injected signal handling")
	}

	logs := h.errs.String()
	for _, want := range []string{"application started", "application work completed", "application stopped"} {
		if !strings.Contains(logs, want) {
			t.Errorf("log output does not contain %q:\n%s", want, logs)
		}
	}
}

func TestServeCommand(t *testing.T) {
	h := newHarness(t, "serve", "--http-host=127.0.0.1", "--http-port=0", "--env=testing")

	if err := h.run(); err != nil {
		t.Fatalf("Run() error = %v, want nil", err)
	}
	logs := h.errs.String()
	for _, want := range []string{"http server listening", "http server draining", "http server stopped", "resources released"} {
		if !strings.Contains(logs, want) {
			t.Errorf("log output does not contain %q:\n%s", want, logs)
		}
	}
}

func TestServeCommandReportsPortInUse(t *testing.T) {
	listener, err := (&net.ListenConfig{}).Listen(context.Background(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve port: %v", err)
	}
	defer listener.Close()

	host, port, err := net.SplitHostPort(listener.Addr().String())
	if err != nil {
		t.Fatalf("split reserved address: %v", err)
	}

	h := newHarness(t, "serve",
		"--http-host="+host,
		"--http-port="+port,
		"--env=testing",
		"--log-level=error",
	)

	err = h.run()
	if err == nil {
		t.Fatal("Run() error = nil, want the startup failure")
	}
	if ExitCode(err) != ExitError {
		t.Errorf("ExitCode(%v) = %d, want %d", err, ExitCode(err), ExitError)
	}
	if !strings.Contains(err.Error(), "listen") {
		t.Errorf("Run() error = %v, want it to mention the failed bind", err)
	}
}

func TestUsageErrors(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{"no_command", nil, "no command given"},
		{"unknown_command", []string{"deploy"}, `unknown command "deploy"`},
		{"unknown_flag", []string{"run", "--nope=1"}, "parse flags"},
		{"positional_argument", []string{"run", "extra"}, "positional"},
		{"env_file_without_value", []string{"run", "--env-file="}, "env-file"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newHarness(t, tt.args...)

			err := h.run()
			if err == nil {
				t.Fatal("Run() error = nil, want a usage error")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Errorf("Run() error = %v, want it to mention %q", err, tt.want)
			}
			if ExitCode(err) != ExitUsage {
				t.Errorf("ExitCode(%v) = %d, want %d", err, ExitCode(err), ExitUsage)
			}
			var usageErr *UsageError
			if !errors.As(err, &usageErr) {
				t.Errorf("Run() error = %T, want a *UsageError", err)
			}
		})
	}
}

func TestUsageErrorsPrintUsageOnStderr(t *testing.T) {
	h := newHarness(t, "deploy")

	if err := h.run(); err == nil {
		t.Fatal("Run() error = nil, want a usage error")
	}
	if !strings.Contains(h.errs.String(), "Usage:") {
		t.Errorf("stderr = %q, want the usage text", h.errs.String())
	}
}

func TestCommandHelpIsNotAnError(t *testing.T) {
	for _, command := range []string{"run", "serve"} {
		t.Run(command, func(t *testing.T) {
			for _, flagArg := range []string{"-h", "--help"} {
				h := newHarness(t, command, flagArg)

				if err := h.run(); err != nil {
					t.Fatalf("Run(%s %s) error = %v, want nil", command, flagArg, err)
				}
				out := h.out.String()
				if !strings.Contains(out, command+" [flags]") {
					t.Errorf("output = %q, want the %s usage", out, command)
				}
				if !strings.Contains(out, "--env-file") || !strings.Contains(out, "--http-port") {
					t.Errorf("output = %q, want the documented flags", out)
				}
			}
		})
	}
}

func TestExitCode(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want int
	}{
		{"success", nil, ExitOK},
		{"usage", usagef("bad input"), ExitUsage},
		{"wrapped_usage", errWrap{usagef("bad input")}, ExitUsage},
		{"runtime", errors.New("boom"), ExitError},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ExitCode(tt.err); got != tt.want {
				t.Errorf("ExitCode(%v) = %d, want %d", tt.err, got, tt.want)
			}
		})
	}
}

func TestNewUsesProcessDefaults(t *testing.T) {
	c := New([]string{programName, "version"}, "9.9.9")

	if c.Out != os.Stdout || c.Err != os.Stderr {
		t.Error("New() must default to the standard streams")
	}
	if c.Getenv == nil || c.SignalContext == nil {
		t.Error("New() must default to the process environment and real signal handling")
	}

	// A CLI without an explicit Getenv must still resolve values.
	c.Getenv = nil
	if got := c.getenv(config.EnvAppName); got != os.Getenv(config.EnvAppName) {
		t.Errorf("getenv() = %q, want %q", got, os.Getenv(config.EnvAppName))
	}
}

func TestEnvFileIsLoadedOutsideProduction(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "custom.env")
	if err := os.WriteFile(path, []byte(config.EnvAppName+"=from-file\n"+config.EnvLogLevel+"=warn\n"), 0o600); err != nil {
		t.Fatalf("write .env: %v", err)
	}

	h := newHarness(t, "run", "--env-file="+path)
	h.env[config.EnvEnvironment] = config.EnvironmentTesting

	if err := h.run(); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if got := h.getenv(config.EnvAppName); got != "from-file" {
		t.Errorf("%s = %q, want from-file", config.EnvAppName, got)
	}
	if !strings.Contains(h.errs.String(), path) {
		t.Errorf("stderr = %q, want a notice about the loaded file", h.errs.String())
	}
}

func TestEnvFileDoesNotOverrideTheEnvironment(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	if err := os.WriteFile(path, []byte(config.EnvAppName+"=from-file\n"), 0o600); err != nil {
		t.Fatalf("write .env: %v", err)
	}

	h := newHarness(t, "run", "--env-file="+path)
	h.env[config.EnvEnvironment] = config.EnvironmentTesting
	h.env[config.EnvAppName] = "from-environment"

	if err := h.run(); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if got := h.getenv(config.EnvAppName); got != "from-environment" {
		t.Errorf("%s = %q, want the environment value to win", config.EnvAppName, got)
	}
}

func TestEnvFileIsRefusedInProduction(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	if err := os.WriteFile(path, []byte(config.EnvAppName+"=from-file\n"), 0o600); err != nil {
		t.Fatalf("write .env: %v", err)
	}

	tests := []struct {
		name string
		args []string
		env  map[string]string
	}{
		{"flag", []string{"run", "--env=production", "--env-file=" + path}, nil},
		{"environment", []string{"run", "--env-file=" + path}, map[string]string{config.EnvEnvironment: config.EnvironmentProduction}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newHarness(t, tt.args...)
			h.env = tt.env

			err := h.run()
			if err == nil {
				t.Fatal("Run() error = nil, want a refusal")
			}
			if !strings.Contains(err.Error(), "production") {
				t.Errorf("Run() error = %v, want it to mention production", err)
			}
			if ExitCode(err) != ExitUsage {
				t.Errorf("ExitCode(%v) = %d, want %d", err, ExitCode(err), ExitUsage)
			}
			if got := h.getenv(config.EnvAppName); got == "from-file" {
				t.Error("the .env file must not be applied in production")
			}
		})
	}
}

func TestEnvFileIsIgnoredInProductionWithoutTheFlag(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	if err := os.WriteFile(path, []byte(config.EnvAppName+"=from-file\n"), 0o600); err != nil {
		t.Fatalf("write .env: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".env.default"), nil, 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	// Run inside the directory that holds a .env file, without asking for it.
	t.Chdir(dir)
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte(config.EnvAppName+"=from-file\n"), 0o600); err != nil {
		t.Fatalf("write .env: %v", err)
	}

	h := newHarness(t, "run", "--env=production")
	h.env = map[string]string{config.EnvAppName: "from-environment"}

	if err := h.run(); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if got := h.getenv(config.EnvAppName); got != "from-environment" {
		t.Errorf("%s = %q, want production to ignore .env", config.EnvAppName, got)
	}
}

func TestMalformedEnvFileIsReported(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	if err := os.WriteFile(path, []byte("this is not a pair\n"), 0o600); err != nil {
		t.Fatalf("write .env: %v", err)
	}

	h := newHarness(t, "run", "--env-file="+path)
	err := h.run()
	if err == nil {
		t.Fatal("Run() error = nil, want the parse failure")
	}
	if ExitCode(err) != ExitUsage {
		t.Errorf("ExitCode(%v) = %d, want %d", err, ExitCode(err), ExitUsage)
	}
}

func TestRunAndServeShareTheSameServiceImplementation(t *testing.T) {
	h := newHarness(t, "run", "--env=testing")
	if err := h.run(); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	deps, help, err := h.cli.prepare("serve", []string{"--http-port=0"})
	if err != nil || help {
		t.Fatalf("prepare() = (%v, %v, %v)", deps, help, err)
	}
	if deps.service == nil {
		t.Fatal("prepare() returned no service")
	}
	if _, err := deps.service.Greet(context.Background(), "ada"); err != nil {
		t.Errorf("Greet() error = %v", err)
	}
}

func TestServeAddressesTheConfiguredHost(t *testing.T) {
	h := newHarness(t, "serve", "--http-host=127.0.0.1", "--http-port=0", "--env=testing")

	if err := h.run(); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	const marker = "http server listening"
	logs := h.errs.String()
	start := strings.Index(logs, marker)
	if start < 0 {
		t.Fatalf("logs do not report the listener:\n%s", logs)
	}
	line := logs[start : strings.Index(logs[start:], "\n")+start]
	if !strings.Contains(line, "127.0.0.1:") {
		t.Errorf("listen log %q does not contain the configured host", line)
	}
	if strings.Contains(line, "127.0.0.1:0") {
		t.Errorf("listen log %q reports the requested port instead of the bound one", line)
	}
}

type errWrap struct{ err error }

func (e errWrap) Error() string { return "wrapped: " + e.err.Error() }
func (e errWrap) Unwrap() error { return e.err }
