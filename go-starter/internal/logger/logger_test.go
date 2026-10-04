package logger

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"strings"
	"testing"

	"github.com/codingseeker/go-starter/internal/config"
)

func testConfig(level, environment string) config.Config {
	cfg := config.Default()
	cfg.LogLevel = level
	cfg.Environment = environment
	return cfg
}

func TestNew_Levels(t *testing.T) {
	for _, level := range []string{"debug", "info", "warn", "error", "INFO", " Warn "} {
		t.Run(level, func(t *testing.T) {
			log, err := New(testConfig(level, config.EnvironmentDevelopment), nil)
			if err != nil {
				t.Fatalf("New() error = %v", err)
			}
			if log == nil {
				t.Fatal("New() returned a nil logger")
			}
		})
	}
}

func TestNew_InvalidLevel(t *testing.T) {
	if _, err := New(testConfig("trace", config.EnvironmentDevelopment), nil); err == nil {
		t.Fatal("New() error = nil, want an error for an unsupported level")
	} else if !strings.Contains(err.Error(), "trace") {
		t.Errorf("New() error = %v, want it to mention the invalid level", err)
	}
}

func TestNew_UsesInjectedWriter(t *testing.T) {
	var buf bytes.Buffer
	log, err := New(testConfig(config.LogLevelInfo, config.EnvironmentDevelopment), &buf)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	log.Info("buffered")
	if buf.Len() == 0 {
		t.Fatal("expected the injected writer to receive the record")
	}
}

func TestNew_DevelopmentUsesReadableText(t *testing.T) {
	var buf bytes.Buffer
	log, err := New(testConfig(config.LogLevelInfo, config.EnvironmentDevelopment), &buf)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	log.Info("hello", "key", "value")

	out := buf.String()
	if strings.HasPrefix(strings.TrimSpace(out), "{") {
		t.Errorf("development output must not be JSON, got %q", out)
	}
	for _, want := range []string{"level=INFO", `msg=hello`, "key=value", "app=" + config.DefaultAppName, "environment=development"} {
		if !strings.Contains(out, want) {
			t.Errorf("development output %q does not contain %q", out, want)
		}
	}
	if !strings.Contains(out, "source=") {
		t.Errorf("development output %q should include the source location", out)
	}
}

func TestNew_ProductionUsesJSON(t *testing.T) {
	var buf bytes.Buffer
	cfg := testConfig(config.LogLevelInfo, config.EnvironmentProduction)
	cfg.AppName = "json-app"
	log, err := New(cfg, &buf)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	log.Info("hello", "key", "value")

	var record map[string]any
	if err := json.Unmarshal(buf.Bytes(), &record); err != nil {
		t.Fatalf("production output is not valid JSON (%v): %s", err, buf.String())
	}
	if record["level"] != "INFO" || record["msg"] != "hello" {
		t.Errorf("unexpected record: %+v", record)
	}
	if record["key"] != "value" {
		t.Errorf("structured field lost: %+v", record)
	}
	if record["app"] != "json-app" || record["environment"] != config.EnvironmentProduction {
		t.Errorf("consistent base fields missing: %+v", record)
	}
	if _, ok := record["source"]; ok {
		t.Errorf("production output should not include source locations: %+v", record)
	}
	if _, ok := record["time"]; !ok {
		t.Errorf("production record should carry a timestamp: %+v", record)
	}
}

func TestNew_RespectsLevel(t *testing.T) {
	tests := []struct {
		level     string
		wantDebug bool
		wantInfo  bool
		wantWarn  bool
		wantError bool
	}{
		{config.LogLevelDebug, true, true, true, true},
		{config.LogLevelInfo, false, true, true, true},
		{config.LogLevelWarn, false, false, true, true},
		{config.LogLevelError, false, false, false, true},
	}

	for _, tt := range tests {
		t.Run(tt.level, func(t *testing.T) {
			var buf bytes.Buffer
			log, err := New(testConfig(tt.level, config.EnvironmentDevelopment), &buf)
			if err != nil {
				t.Fatalf("New() error = %v", err)
			}
			log.Debug("debug-message")
			log.Info("info-message")
			log.Warn("warn-message")
			log.Error("error-message")

			out := buf.String()
			for _, tc := range []struct {
				name string
				want bool
			}{
				{"debug-message", tt.wantDebug},
				{"info-message", tt.wantInfo},
				{"warn-message", tt.wantWarn},
				{"error-message", tt.wantError},
			} {
				if got := strings.Contains(out, tc.name); got != tc.want {
					t.Errorf("level %s: %s logged = %v, want %v (output: %s)", tt.level, tc.name, got, tc.want, out)
				}
			}
		})
	}
}

func TestParseLevel(t *testing.T) {
	tests := []struct {
		level   string
		want    slog.Level
		wantErr bool
	}{
		{"debug", slog.LevelDebug, false},
		{"info", slog.LevelInfo, false},
		{"WARN", slog.LevelWarn, false},
		{" error ", slog.LevelError, false},
		{"", 0, true},
		{"fatal", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.level, func(t *testing.T) {
			got, err := ParseLevel(tt.level)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("ParseLevel(%q) error = nil, want an error", tt.level)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseLevel(%q) error = %v", tt.level, err)
			}
			if got != tt.want {
				t.Errorf("ParseLevel(%q) = %v, want %v", tt.level, got, tt.want)
			}
		})
	}
}

func TestContextRoundTrip(t *testing.T) {
	var buf bytes.Buffer
	log, err := New(testConfig(config.LogLevelInfo, config.EnvironmentDevelopment), &buf)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	ctx := WithContext(context.Background(), log.With(slog.String("request_id", "abc123")))
	FromContext(ctx).InfoContext(ctx, "handled")

	if !strings.Contains(buf.String(), "request_id=abc123") {
		t.Errorf("context logger lost its attributes: %s", buf.String())
	}
}

func TestFromContextFallbacks(t *testing.T) {
	if FromContext(context.Background()) == nil {
		t.Error("FromContext(background) = nil, want slog.Default()")
	}
	// A context without a stored logger falls back to the standard logger.
	if FromContext(context.TODO()) == nil {
		t.Error("FromContext(context.TODO()) = nil, want slog.Default()")
	}
	if ctx := WithContext(context.Background(), nil); ctx == nil {
		t.Error("WithContext(ctx, nil) must not panic")
	}
}

func TestFromContextOrPrefersTheContextLogger(t *testing.T) {
	fallback, err := New(testConfig(config.LogLevelInfo, config.EnvironmentDevelopment), io.Discard)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	ctxLogger := slog.New(slog.NewJSONHandler(io.Discard, nil))

	if got := FromContextOr(context.Background(), fallback); got != fallback {
		t.Error("FromContextOr(background, fallback) did not return the fallback logger")
	}
	if got := FromContextOr(WithContext(context.Background(), ctxLogger), fallback); got != ctxLogger {
		t.Error("FromContextOr(ctx with logger, fallback) did not return the context logger")
	}
	if got := FromContextOr(context.Background(), nil); got != slog.Default() {
		t.Error("FromContextOr(ctx, nil) did not fall back to slog.Default()")
	}
}

func TestNoopDiscardsEverything(t *testing.T) {
	log := Noop()
	if log == nil {
		t.Fatal("Noop() = nil")
	}
	ctx := context.Background()
	for _, level := range []slog.Level{slog.LevelDebug, slog.LevelInfo, slog.LevelWarn, slog.LevelError} {
		if log.Enabled(ctx, level) {
			t.Errorf("Noop logger must drop %s records", level)
		}
		log.Log(ctx, level, "dropped")
	}
	if !log.Enabled(ctx, slog.LevelError+1) {
		t.Error("Noop logger must not report itself as disabled for every level")
	}
}
