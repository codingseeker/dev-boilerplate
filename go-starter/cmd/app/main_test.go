package main

import (
	"bytes"
	"context"
	"io"
	"os"
	"strings"
	"testing"
)

// runCLI executes the entry point with in-memory output and returns the exit
// code together with everything the process printed.
func runCLI(t *testing.T, args ...string) (int, string, string) {
	t.Helper()

	var stdout, stderr bytes.Buffer
	code := run(context.Background(), args, &stdout, &stderr)
	return code, stdout.String(), stderr.String()
}

func TestRunExitCodes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		args     []string
		wantCode int
		wantOut  string
		wantErr  string
	}{
		{
			name:     "help succeeds",
			args:     []string{"app", "--help"},
			wantCode: 0,
			wantOut:  "Commands:",
		},
		{
			name:     "version succeeds",
			args:     []string{"app", "version"},
			wantCode: 0,
			wantOut:  "",
		},
		{
			name:     "unknown command is a usage error",
			args:     []string{"app", "nope"},
			wantCode: 2,
			wantErr:  "unknown command",
		},
		{
			name:     "unknown flag is a usage error",
			args:     []string{"app", "serve", "--unknown"},
			wantCode: 2,
			wantErr:  "not defined",
		},
		{
			name:     "invalid configuration is a usage error",
			args:     []string{"app", "serve", "--log-level=shout"},
			wantCode: 2,
			wantErr:  "invalid",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			code, stdout, stderr := runCLI(t, tc.args...)
			if code != tc.wantCode {
				t.Errorf("run(%q) = %d, want %d (stderr: %q)", tc.args, code, tc.wantCode, stderr)
			}
			if tc.wantOut != "" && !strings.Contains(stdout, tc.wantOut) {
				t.Errorf("stdout = %q, want it to contain %q", stdout, tc.wantOut)
			}
			if tc.wantErr != "" && !strings.Contains(stderr, tc.wantErr) {
				t.Errorf("stderr = %q, want it to contain %q", stderr, tc.wantErr)
			}
		})
	}
}

func TestRunVersionOutputIsNotEmpty(t *testing.T) {
	t.Parallel()

	code, stdout, stderr := runCLI(t, "app", "version")
	if code != 0 {
		t.Fatalf("run(version) = %d, want 0 (stderr: %q)", code, stderr)
	}
	if strings.TrimSpace(stdout) == "" {
		t.Error("run(version) printed no version information")
	}
}

func TestRunCancelledContextStopsGracefully(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	var stdout, stderr bytes.Buffer
	code := run(ctx, []string{"app", "run"}, &stdout, &stderr)
	if code != 0 {
		t.Errorf("run with a cancelled context = %d, want 0 (stderr: %q)", code, stderr.String())
	}
}

func TestRunWritesErrorsToStderrOnly(t *testing.T) {
	t.Parallel()

	code, stdout, stderr := runCLI(t, "app", "nope")
	if code != 2 {
		t.Fatalf("run(unknown command) = %d, want 2", code)
	}
	if strings.TrimSpace(stderr) == "" {
		t.Error("run(unknown command) wrote no error to stderr")
	}
	if strings.Contains(stdout, "error") {
		t.Errorf("stdout = %q, want no diagnostics on stdout", stdout)
	}
}

func TestMainUsesRealProcessFiles(t *testing.T) {
	// main itself is a three line wrapper; the behaviour lives in run, which is
	// covered above. This test only asserts that the signatures stay compatible
	// with os.Exit and the process entry point.
	t.Parallel()

	var stdout, stderr bytes.Buffer
	if code := run(context.Background(), []string{"app", "help"}, &stdout, &stderr); code != 0 {
		t.Fatalf("run(help) = %d, want 0", code)
	}
	if !strings.Contains(stdout.String(), "Usage:") {
		t.Errorf("stdout = %q, want it to contain the usage text", stdout.String())
	}

	// os.Stdout and os.Stderr must satisfy the writer parameters of run.
	var _ io.Writer = os.Stdout
	var _ io.Writer = os.Stderr
}
