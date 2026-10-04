// Package domain holds the core business types and the contracts the
// application layer exposes. It must stay free of CLI and HTTP concerns so the
// same logic can be driven from any transport.
package domain

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// Readiness states reported by Health and Ready.
const (
	StatusOK       = "ok"
	StatusDegraded = "degraded"
)

// MaxNameLength bounds user supplied names accepted by Greet.
const MaxNameLength = 64

// ErrInvalidName is returned when a caller supplies an unusable name.
var ErrInvalidName = errors.New("invalid name")

// HealthStatus describes the state of the process and of its dependencies.
type HealthStatus struct {
	Status      string            `json:"status"`
	AppName     string            `json:"app"`
	Environment string            `json:"environment"`
	Version     string            `json:"version"`
	Checks      map[string]string `json:"checks,omitempty"`
}

// OK reports whether every check passed.
func (h HealthStatus) OK() bool {
	return h.Status == StatusOK
}

// Greeting is the result of the demonstration business operation.
type Greeting struct {
	Name    string `json:"name"`
	Message string `json:"message"`
}

// HealthChecker reports liveness and readiness of the application.
type HealthChecker interface {
	Health(ctx context.Context) (HealthStatus, error)
	Ready(ctx context.Context) (HealthStatus, error)
}

// Greeter is the demonstration business capability. It is the smallest unit
// that still proves the architecture: business logic lives behind an interface
// that transports depend on, not on concrete implementations.
type Greeter interface {
	Greet(ctx context.Context, name string) (Greeting, error)
}

// Service is the full application capability set required by the CLI and the
// HTTP API.
type Service interface {
	HealthChecker
	Greeter
}

// NormalizeName validates and trims a caller supplied name.
func NormalizeName(name string) (string, error) {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return "", fmt.Errorf("%w: name must not be empty", ErrInvalidName)
	}
	if len(trimmed) > MaxNameLength {
		return "", fmt.Errorf("%w: name must be at most %d characters", ErrInvalidName, MaxNameLength)
	}
	return trimmed, nil
}
