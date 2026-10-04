// Package config resolves and validates the application configuration from
// environment variables, CLI flags or dotenv files.
package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Supported environments.
const (
	EnvironmentDevelopment = "development"
	EnvironmentTesting     = "testing"
	EnvironmentProduction  = "production"
)

// Supported log levels.
const (
	LogLevelDebug = "debug"
	LogLevelInfo  = "info"
	LogLevelWarn  = "warn"
	LogLevelError = "error"
)

// Defaults applied when neither a CLI flag nor an environment variable is set.
const (
	DefaultAppName         = "go-starter"
	DefaultEnvironment     = EnvironmentDevelopment
	DefaultLogLevel        = LogLevelInfo
	DefaultHTTPHost        = "0.0.0.0"
	DefaultHTTPPort        = 8080
	DefaultReadTimeout     = 30 * time.Second
	DefaultWriteTimeout    = 30 * time.Second
	DefaultIdleTimeout     = 120 * time.Second
	DefaultShutdownTimeout = 30 * time.Second
)

// Environment variable names read by LoadFromEnv.
const (
	EnvAppName         = "APP_NAME"
	EnvEnvironment     = "APP_ENV"
	EnvLogLevel        = "LOG_LEVEL"
	EnvHTTPHost        = "HTTP_HOST"
	EnvHTTPPort        = "HTTP_PORT"
	EnvReadTimeout     = "HTTP_READ_TIMEOUT"
	EnvWriteTimeout    = "HTTP_WRITE_TIMEOUT"
	EnvIdleTimeout     = "HTTP_IDLE_TIMEOUT"
	EnvShutdownTimeout = "HTTP_SHUTDOWN_TIMEOUT"
)

// Keys lists every environment variable understood by LoadFromEnv, in
// declaration order.
func Keys() []string {
	return []string{
		EnvAppName,
		EnvEnvironment,
		EnvLogLevel,
		EnvHTTPHost,
		EnvHTTPPort,
		EnvReadTimeout,
		EnvWriteTimeout,
		EnvIdleTimeout,
		EnvShutdownTimeout,
	}
}

// Config holds the fully resolved application configuration.
type Config struct {
	AppName     string
	Environment string
	LogLevel    string
	HTTP        HTTPConfig
}

// HTTPConfig holds HTTP server configuration.
type HTTPConfig struct {
	Host            string
	Port            int
	ReadTimeout     time.Duration
	WriteTimeout    time.Duration
	IdleTimeout     time.Duration
	ShutdownTimeout time.Duration
}

// Addr returns the listen address of the HTTP server.
func (h HTTPConfig) Addr() string {
	return fmt.Sprintf("%s:%d", h.Host, h.Port)
}

// IsProduction reports whether the application runs in production mode.
func (c Config) IsProduction() bool {
	return c.Environment == EnvironmentProduction
}

// Default returns the configuration used when nothing is overridden.
func Default() Config {
	return Config{
		AppName:     DefaultAppName,
		Environment: DefaultEnvironment,
		LogLevel:    DefaultLogLevel,
		HTTP: HTTPConfig{
			Host:            DefaultHTTPHost,
			Port:            DefaultHTTPPort,
			ReadTimeout:     DefaultReadTimeout,
			WriteTimeout:    DefaultWriteTimeout,
			IdleTimeout:     DefaultIdleTimeout,
			ShutdownTimeout: DefaultShutdownTimeout,
		},
	}
}

// Validate checks that configuration is usable, reporting the first problem
// found together with the offending value.
func (c Config) Validate() error {
	if strings.TrimSpace(c.AppName) == "" {
		return fmt.Errorf("invalid %s: value must not be empty", EnvAppName)
	}
	if !isOneOf(c.Environment, EnvironmentDevelopment, EnvironmentTesting, EnvironmentProduction) {
		return fmt.Errorf("invalid %s %q: must be %s, %s or %s",
			EnvEnvironment, c.Environment, EnvironmentDevelopment, EnvironmentTesting, EnvironmentProduction)
	}
	if !isOneOf(c.LogLevel, LogLevelDebug, LogLevelInfo, LogLevelWarn, LogLevelError) {
		return fmt.Errorf("invalid %s %q: must be %s, %s, %s or %s",
			EnvLogLevel, c.LogLevel, LogLevelDebug, LogLevelInfo, LogLevelWarn, LogLevelError)
	}
	if strings.TrimSpace(c.HTTP.Host) == "" {
		return fmt.Errorf("invalid %s: value must not be empty", EnvHTTPHost)
	}
	if c.HTTP.Port < 1 || c.HTTP.Port > 65535 {
		return fmt.Errorf("invalid %s %d: must be between 1 and 65535", EnvHTTPPort, c.HTTP.Port)
	}
	if err := validatePositiveDuration(EnvReadTimeout, c.HTTP.ReadTimeout); err != nil {
		return err
	}
	if err := validatePositiveDuration(EnvWriteTimeout, c.HTTP.WriteTimeout); err != nil {
		return err
	}
	if err := validatePositiveDuration(EnvIdleTimeout, c.HTTP.IdleTimeout); err != nil {
		return err
	}
	return validatePositiveDuration(EnvShutdownTimeout, c.HTTP.ShutdownTimeout)
}

// Load reads the configuration from the process environment.
func Load() (Config, error) {
	return LoadFromEnv(os.Getenv)
}

// LoadFromEnv resolves the configuration from getenv, applying defaults for
// unset or blank values and validating the result. Passing nil uses
// os.Getenv. Callers that layer extra sources on top of the environment (CLI
// flags or a dotenv file) pass their own lookup function, which keeps the
// precedence rules testable without touching global state.
func LoadFromEnv(getenv func(string) string) (Config, error) {
	if getenv == nil {
		getenv = os.Getenv
	}

	cfg := Default()

	if v := strings.TrimSpace(getenv(EnvAppName)); v != "" {
		cfg.AppName = v
	}
	if v := strings.ToLower(strings.TrimSpace(getenv(EnvEnvironment))); v != "" {
		cfg.Environment = v
	}
	if v := strings.ToLower(strings.TrimSpace(getenv(EnvLogLevel))); v != "" {
		cfg.LogLevel = v
	}
	if v := strings.TrimSpace(getenv(EnvHTTPHost)); v != "" {
		cfg.HTTP.Host = v
	}
	if v := strings.TrimSpace(getenv(EnvHTTPPort)); v != "" {
		port, err := parsePort(EnvHTTPPort, v)
		if err != nil {
			return Config{}, err
		}
		cfg.HTTP.Port = port
	}

	durations := []struct {
		key  string
		dest *time.Duration
	}{
		{EnvReadTimeout, &cfg.HTTP.ReadTimeout},
		{EnvWriteTimeout, &cfg.HTTP.WriteTimeout},
		{EnvIdleTimeout, &cfg.HTTP.IdleTimeout},
		{EnvShutdownTimeout, &cfg.HTTP.ShutdownTimeout},
	}
	for _, item := range durations {
		v := strings.TrimSpace(getenv(item.key))
		if v == "" {
			continue
		}
		d, err := parseDuration(item.key, v)
		if err != nil {
			return Config{}, err
		}
		*item.dest = d
	}

	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func parsePort(key, raw string) (int, error) {
	port, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("invalid %s %q: must be an integer between 1 and 65535", key, raw)
	}
	return port, nil
}

// parseDuration accepts Go durations (30s, 1m30s) and bare integers that are
// interpreted as seconds, which keeps container friendly values such as
// HTTP_IDLE_TIMEOUT=60 working.
func parseDuration(key, raw string) (time.Duration, error) {
	if d, err := time.ParseDuration(raw); err == nil {
		return d, nil
	}
	if seconds, err := strconv.Atoi(raw); err == nil {
		return time.Duration(seconds) * time.Second, nil
	}
	return 0, fmt.Errorf("invalid %s %q: must be a duration such as 30s or 1m30s, or a whole number of seconds", key, raw)
}

func validatePositiveDuration(key string, d time.Duration) error {
	if d <= 0 {
		return fmt.Errorf("invalid %s %s: must be greater than zero", key, d)
	}
	return nil
}

func isOneOf(value string, allowed ...string) bool {
	for _, candidate := range allowed {
		if value == candidate {
			return true
		}
	}
	return false
}
