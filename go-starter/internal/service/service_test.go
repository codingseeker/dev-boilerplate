package service

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/codingseeker/go-starter/internal/buildinfo"
	"github.com/codingseeker/go-starter/internal/config"
	"github.com/codingseeker/go-starter/internal/domain"
)

func newTestService(t *testing.T, environment string) *ApplicationService {
	t.Helper()
	cfg := config.Default()
	cfg.AppName = "test-app"
	cfg.Environment = environment
	return New(cfg)
}

func TestHealth(t *testing.T) {
	svc := newTestService(t, config.EnvironmentTesting)

	status, err := svc.Health(context.Background())
	if err != nil {
		t.Fatalf("Health() error = %v", err)
	}
	if status.Status != domain.StatusOK {
		t.Errorf("Status = %q, want %q", status.Status, domain.StatusOK)
	}
	if status.AppName != "test-app" {
		t.Errorf("AppName = %q, want %q", status.AppName, "test-app")
	}
	if status.Environment != config.EnvironmentTesting {
		t.Errorf("Environment = %q, want %q", status.Environment, config.EnvironmentTesting)
	}
	if status.Version != buildinfo.Version {
		t.Errorf("Version = %q, want %q", status.Version, buildinfo.Version)
	}
	if status.Checks["config"] != "ok" {
		t.Errorf("Checks = %v, want a config check", status.Checks)
	}
}

func TestReady(t *testing.T) {
	svc := newTestService(t, config.EnvironmentDevelopment)

	status, err := svc.Ready(context.Background())
	if err != nil {
		t.Fatalf("Ready() error = %v", err)
	}
	if !status.OK() {
		t.Errorf("Ready() status = %q, want %q", status.Status, domain.StatusOK)
	}
	if status.Checks["service"] != "ok" {
		t.Errorf("Checks = %v, want a service check", status.Checks)
	}
}

func TestReadyWhileShuttingDown(t *testing.T) {
	svc := newTestService(t, config.EnvironmentProduction)
	if err := svc.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	// Close is idempotent.
	if err := svc.Close(); err != nil {
		t.Fatalf("second Close() error = %v", err)
	}

	status, err := svc.Ready(context.Background())
	if err == nil {
		t.Fatal("Ready() error = nil, want an error while shutting down")
	}
	if status.OK() {
		t.Errorf("status = %q, want %q while shutting down", status.Status, domain.StatusDegraded)
	}
	if status.Checks["service"] != "shutting down" {
		t.Errorf("Checks = %v, want the service check to report the shutdown", status.Checks)
	}

	// Liveness stays green so a draining instance is not restarted.
	if health, err := svc.Health(context.Background()); err != nil || !health.OK() {
		t.Errorf("Health() during shutdown = (%+v, %v), want a healthy status", health, err)
	}
}

func TestGreet(t *testing.T) {
	svc := newTestService(t, config.EnvironmentTesting)

	greeting, err := svc.Greet(context.Background(), "  ada ")
	if err != nil {
		t.Fatalf("Greet() error = %v", err)
	}
	if greeting.Name != "ada" {
		t.Errorf("Name = %q, want %q", greeting.Name, "ada")
	}
	for _, want := range []string{"ada", "test-app", config.EnvironmentTesting} {
		if !strings.Contains(greeting.Message, want) {
			t.Errorf("Message = %q, want it to mention %q", greeting.Message, want)
		}
	}
}

func TestGreetInvalidName(t *testing.T) {
	svc := newTestService(t, config.EnvironmentDevelopment)

	for _, name := range []string{"", "   ", strings.Repeat("x", domain.MaxNameLength+1)} {
		if _, err := svc.Greet(context.Background(), name); err == nil {
			t.Errorf("Greet(%q) error = nil, want an error", name)
		} else if !errors.Is(err, domain.ErrInvalidName) {
			t.Errorf("Greet(%q) error = %v, want it to wrap ErrInvalidName", name, err)
		}
	}
}

func TestGreetRespectsContextCancellation(t *testing.T) {
	svc := newTestService(t, config.EnvironmentDevelopment)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := svc.Greet(ctx, "ada"); !errors.Is(err, context.Canceled) {
		t.Fatalf("Greet() error = %v, want context.Canceled", err)
	}
}

func TestConcurrentUseIsSafe(t *testing.T) {
	svc := newTestService(t, config.EnvironmentDevelopment)
	const goroutines = 16

	done := make(chan error, goroutines)
	for i := 0; i < goroutines; i++ {
		go func() {
			if _, err := svc.Greet(context.Background(), "ada"); err != nil {
				done <- err
				return
			}
			_, err := svc.Ready(context.Background())
			done <- err
		}()
	}
	for i := 0; i < goroutines; i++ {
		if err := <-done; err != nil {
			t.Fatalf("concurrent call error = %v", err)
		}
	}
}
