// Package logger builds the single structured logger of the application and
// carries it through context.Context.
package logger

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"

	"github.com/codingseeker/go-starter/internal/config"
)

type contextKey struct{}

// New builds the single application logger. Human readable text output is used
// outside production, JSON output in production. A nil writer defaults to
// os.Stdout. Every record carries the application name and environment so log
// aggregation does not depend on message text.
func New(cfg config.Config, w io.Writer) (*slog.Logger, error) {
	logLevel, err := ParseLevel(cfg.LogLevel)
	if err != nil {
		return nil, err
	}
	if w == nil {
		w = os.Stdout
	}

	production := cfg.IsProduction()
	handlerOptions := &slog.HandlerOptions{Level: logLevel, AddSource: !production}

	var handler slog.Handler
	if production {
		handler = slog.NewJSONHandler(w, handlerOptions)
	} else {
		handler = slog.NewTextHandler(w, handlerOptions)
	}

	return slog.New(handler).With(
		slog.String("app", cfg.AppName),
		slog.String("environment", cfg.Environment),
	), nil
}

// ParseLevel maps a configuration log level onto a slog level.
func ParseLevel(level string) (slog.Level, error) {
	switch strings.ToLower(strings.TrimSpace(level)) {
	case config.LogLevelDebug:
		return slog.LevelDebug, nil
	case config.LogLevelInfo:
		return slog.LevelInfo, nil
	case config.LogLevelWarn:
		return slog.LevelWarn, nil
	case config.LogLevelError:
		return slog.LevelError, nil
	default:
		return 0, fmt.Errorf("unsupported log level %q: must be %s, %s, %s or %s",
			level, config.LogLevelDebug, config.LogLevelInfo, config.LogLevelWarn, config.LogLevelError)
	}
}

// WithContext returns a context carrying log, so downstream layers can enrich
// the record set with request scoped fields without extra parameters.
func WithContext(ctx context.Context, log *slog.Logger) context.Context {
	if log == nil {
		return ctx
	}
	return context.WithValue(ctx, contextKey{}, log)
}

// FromContext returns the logger stored in ctx, or slog.Default when absent.
func FromContext(ctx context.Context) *slog.Logger {
	if ctx != nil {
		if log, ok := ctx.Value(contextKey{}).(*slog.Logger); ok && log != nil {
			return log
		}
	}
	return slog.Default()
}

// FromContextOr returns the logger stored in ctx, falling back to the given
// logger. Layers that own a logger use it so they work without help from their
// caller, while a context logger still wins when present.
func FromContextOr(ctx context.Context, fallback *slog.Logger) *slog.Logger {
	if ctx != nil {
		if log, ok := ctx.Value(contextKey{}).(*slog.Logger); ok && log != nil {
			return log
		}
	}
	if fallback != nil {
		return fallback
	}
	return slog.Default()
}

// Noop returns a logger that discards everything below error level. It keeps
// tests quiet without changing call sites.
func Noop() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelError + 1}))
}
