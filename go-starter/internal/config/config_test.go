package config

import (
	"strconv"
	"strings"
	"testing"
	"time"
)

func envFrom(values map[string]string) func(string) string {
	return func(key string) string { return values[key] }
}

func TestDefault(t *testing.T) {
	cfg := Default()

	if err := cfg.Validate(); err != nil {
		t.Fatalf("default configuration must be valid, got %v", err)
	}
	if cfg.AppName != DefaultAppName {
		t.Errorf("AppName = %q, want %q", cfg.AppName, DefaultAppName)
	}
	if cfg.Environment != DefaultEnvironment {
		t.Errorf("Environment = %q, want %q", cfg.Environment, DefaultEnvironment)
	}
	if cfg.LogLevel != DefaultLogLevel {
		t.Errorf("LogLevel = %q, want %q", cfg.LogLevel, DefaultLogLevel)
	}
	if got, want := cfg.HTTP.Addr(), "0.0.0.0:8080"; got != want {
		t.Errorf("Addr() = %q, want %q", got, want)
	}
	if cfg.IsProduction() {
		t.Error("IsProduction() = true for the development default")
	}
}

func TestLoadFromEnv_Defaults(t *testing.T) {
	cfg, err := LoadFromEnv(envFrom(nil))
	if err != nil {
		t.Fatalf("LoadFromEnv() error = %v", err)
	}
	if cfg != Default() {
		t.Errorf("configuration with an empty environment = %+v, want %+v", cfg, Default())
	}
}

func TestLoadFromEnv_ValidValues(t *testing.T) {
	cfg, err := LoadFromEnv(envFrom(map[string]string{
		EnvAppName:         "myapp",
		EnvEnvironment:     "production",
		EnvLogLevel:        "debug",
		EnvHTTPHost:        "127.0.0.1",
		EnvHTTPPort:        "9000",
		EnvReadTimeout:     "5s",
		EnvWriteTimeout:    "10s",
		EnvIdleTimeout:     "1m30s",
		EnvShutdownTimeout: "15s",
	}))
	if err != nil {
		t.Fatalf("LoadFromEnv() error = %v", err)
	}

	if cfg.AppName != "myapp" {
		t.Errorf("AppName = %q, want %q", cfg.AppName, "myapp")
	}
	if !cfg.IsProduction() {
		t.Error("IsProduction() = false, want true")
	}
	if cfg.LogLevel != LogLevelDebug {
		t.Errorf("LogLevel = %q, want %q", cfg.LogLevel, LogLevelDebug)
	}
	if got, want := cfg.HTTP.Addr(), "127.0.0.1:9000"; got != want {
		t.Errorf("Addr() = %q, want %q", got, want)
	}
	if cfg.HTTP.IdleTimeout != 90*time.Second {
		t.Errorf("IdleTimeout = %s, want 1m30s", cfg.HTTP.IdleTimeout)
	}
}

func TestLoadFromEnv_TrimsAndNormalises(t *testing.T) {
	cfg, err := LoadFromEnv(envFrom(map[string]string{
		EnvAppName:     "  spaced  ",
		EnvEnvironment: "  PRODUCTION ",
		EnvLogLevel:    " WARN ",
		EnvHTTPHost:    " localhost ",
		EnvHTTPPort:    " 8081 ",
	}))
	if err != nil {
		t.Fatalf("LoadFromEnv() error = %v", err)
	}
	if cfg.AppName != "spaced" {
		t.Errorf("AppName = %q, want %q", cfg.AppName, "spaced")
	}
	if cfg.Environment != EnvironmentProduction {
		t.Errorf("Environment = %q, want %q", cfg.Environment, EnvironmentProduction)
	}
	if cfg.LogLevel != LogLevelWarn {
		t.Errorf("LogLevel = %q, want %q", cfg.LogLevel, LogLevelWarn)
	}
	if cfg.HTTP.Host != "localhost" || cfg.HTTP.Port != 8081 {
		t.Errorf("HTTP host/port = %q/%d, want localhost/8081", cfg.HTTP.Host, cfg.HTTP.Port)
	}
}

func TestLoadFromEnv_BlankValuesFallBackToDefaults(t *testing.T) {
	blank := strings.Repeat(" ", 8)
	cfg, err := LoadFromEnv(envFrom(map[string]string{
		EnvAppName:     blank,
		EnvEnvironment: blank,
		EnvLogLevel:    blank,
		EnvHTTPHost:    blank,
		EnvHTTPPort:    blank,
		EnvReadTimeout: blank,
	}))
	if err != nil {
		t.Fatalf("LoadFromEnv() error = %v", err)
	}
	if cfg != Default() {
		t.Errorf("blank values must fall back to defaults, got %+v", cfg)
	}
}

func TestLoadFromEnv_Override(t *testing.T) {
	// A layer on top of the environment (for example CLI flags) must win.
	base := envFrom(map[string]string{
		EnvAppName:     "from-env",
		EnvEnvironment: EnvironmentTesting,
		EnvLogLevel:    LogLevelWarn,
		EnvHTTPPort:    "8081",
	})
	values := map[string]string{
		EnvAppName:     "from-flag",
		EnvEnvironment: EnvironmentProduction,
		EnvLogLevel:    LogLevelError,
		EnvHTTPPort:    "9999",
	}

	cfg, err := LoadFromEnv(func(key string) string {
		if value, ok := values[key]; ok {
			return value
		}
		return base(key)
	})
	if err != nil {
		t.Fatalf("LoadFromEnv() error = %v", err)
	}
	if cfg.AppName != "from-flag" {
		t.Errorf("AppName = %q, want %q", cfg.AppName, "from-flag")
	}
	if cfg.Environment != EnvironmentProduction {
		t.Errorf("Environment = %q, want %q", cfg.Environment, EnvironmentProduction)
	}
	if cfg.LogLevel != LogLevelError {
		t.Errorf("LogLevel = %q, want %q", cfg.LogLevel, LogLevelError)
	}
	if cfg.HTTP.Port != 9999 {
		t.Errorf("HTTP.Port = %d, want 9999", cfg.HTTP.Port)
	}
}

func TestLoadFromEnv_DurationAcceptsBareSeconds(t *testing.T) {
	cfg, err := LoadFromEnv(envFrom(map[string]string{EnvReadTimeout: "10"}))
	if err != nil {
		t.Fatalf("LoadFromEnv() error = %v", err)
	}
	if cfg.HTTP.ReadTimeout != 10*time.Second {
		t.Errorf("ReadTimeout = %s, want 10s", cfg.HTTP.ReadTimeout)
	}
}

func TestLoadFromEnv_InvalidEnvironment(t *testing.T) {
	_, err := LoadFromEnv(envFrom(map[string]string{EnvEnvironment: "staging"}))
	if err == nil {
		t.Fatal("LoadFromEnv() error = nil, want an APP_ENV error")
	}
	if !strings.Contains(err.Error(), EnvEnvironment) || !strings.Contains(err.Error(), "staging") {
		t.Errorf("error = %v, want it to mention %s and the invalid value", err, EnvEnvironment)
	}
}

func TestLoadFromEnv_InvalidLogLevel(t *testing.T) {
	_, err := LoadFromEnv(envFrom(map[string]string{EnvLogLevel: "verbose"}))
	if err == nil {
		t.Fatal("LoadFromEnv() error = nil, want a LOG_LEVEL error")
	}
	if !strings.Contains(err.Error(), EnvLogLevel) || !strings.Contains(err.Error(), "verbose") {
		t.Errorf("error = %v, want it to mention %s and the invalid value", err, EnvLogLevel)
	}
}

func TestLoadFromEnv_InvalidValues(t *testing.T) {
	tests := []struct {
		name    string
		env     map[string]string
		wantKey string
	}{
		{"port_not_a_number", map[string]string{EnvHTTPPort: "http"}, EnvHTTPPort},
		{"port_zero", map[string]string{EnvHTTPPort: "0"}, EnvHTTPPort},
		{"port_too_high", map[string]string{EnvHTTPPort: "70000"}, EnvHTTPPort},
		{"read_timeout_zero", map[string]string{EnvReadTimeout: "0s"}, EnvReadTimeout},
		{"write_timeout_negative", map[string]string{EnvWriteTimeout: "-1s"}, EnvWriteTimeout},
		{"idle_timeout_zero", map[string]string{EnvIdleTimeout: "0"}, EnvIdleTimeout},
		{"shutdown_timeout_garbage", map[string]string{EnvShutdownTimeout: "bad"}, EnvShutdownTimeout},
		{"shutdown_timeout_negative", map[string]string{EnvShutdownTimeout: "-5"}, EnvShutdownTimeout},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := LoadFromEnv(envFrom(tt.env))
			if err == nil {
				t.Fatal("LoadFromEnv() error = nil, want an error")
			}
			if !strings.Contains(err.Error(), tt.wantKey) {
				t.Errorf("error = %v, want it to mention %s", err, tt.wantKey)
			}
		})
	}
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*Config)
		wantErr string
	}{
		{"valid_defaults", func(*Config) {}, ""},
		{"valid_testing", func(c *Config) { c.Environment = EnvironmentTesting }, ""},
		{"empty_app_name", func(c *Config) { c.AppName = "   " }, EnvAppName},
		{"empty_host", func(c *Config) { c.HTTP.Host = "" }, EnvHTTPHost},
		{"bad_environment", func(c *Config) { c.Environment = "prod" }, EnvEnvironment},
		{"bad_log_level", func(c *Config) { c.LogLevel = "trace" }, EnvLogLevel},
		{"bad_port", func(c *Config) { c.HTTP.Port = -1 }, EnvHTTPPort},
		{"zero_read_timeout", func(c *Config) { c.HTTP.ReadTimeout = 0 }, EnvReadTimeout},
		{"zero_write_timeout", func(c *Config) { c.HTTP.WriteTimeout = 0 }, EnvWriteTimeout},
		{"zero_idle_timeout", func(c *Config) { c.HTTP.IdleTimeout = 0 }, EnvIdleTimeout},
		{"zero_shutdown_timeout", func(c *Config) { c.HTTP.ShutdownTimeout = 0 }, EnvShutdownTimeout},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := Default()
			tt.mutate(&cfg)

			err := cfg.Validate()
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("Validate() error = %v, want nil", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("Validate() error = nil, want an error mentioning %s", tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("Validate() error = %v, want it to mention %s", err, tt.wantErr)
			}
		})
	}
}

func TestKeysCoversEveryDocumentedVariable(t *testing.T) {
	keys := Keys()
	if len(keys) != 9 {
		t.Errorf("Keys() returned %d entries, want 9", len(keys))
	}

	// Every key must be honoured by LoadFromEnv: setting it to a distinctive
	// value must change the resolved configuration.
	for _, key := range keys {
		t.Run(key, func(t *testing.T) {
			cfg, err := LoadFromEnv(envFrom(map[string]string{key: distinctiveValue(key)}))
			if err != nil {
				t.Fatalf("LoadFromEnv() error = %v", err)
			}
			if got := valueOf(cfg, key); got != distinctiveValue(key) {
				t.Errorf("%s resolved to %q, want %q", key, got, distinctiveValue(key))
			}
		})
	}
}

func distinctiveValue(key string) string {
	switch key {
	case EnvAppName:
		return "key-probe-app"
	case EnvEnvironment:
		return EnvironmentTesting
	case EnvLogLevel:
		return LogLevelError
	case EnvHTTPHost:
		return "127.0.0.1"
	case EnvHTTPPort:
		return "8123"
	case EnvReadTimeout:
		return "11s"
	case EnvWriteTimeout:
		return "12s"
	case EnvIdleTimeout:
		return "13s"
	case EnvShutdownTimeout:
		return "14s"
	default:
		return ""
	}
}

func valueOf(cfg Config, key string) string {
	switch key {
	case EnvAppName:
		return cfg.AppName
	case EnvEnvironment:
		return cfg.Environment
	case EnvLogLevel:
		return cfg.LogLevel
	case EnvHTTPHost:
		return cfg.HTTP.Host
	case EnvHTTPPort:
		return strconv.Itoa(cfg.HTTP.Port)
	case EnvReadTimeout:
		return cfg.HTTP.ReadTimeout.String()
	case EnvWriteTimeout:
		return cfg.HTTP.WriteTimeout.String()
	case EnvIdleTimeout:
		return cfg.HTTP.IdleTimeout.String()
	case EnvShutdownTimeout:
		return cfg.HTTP.ShutdownTimeout.String()
	default:
		return ""
	}
}
