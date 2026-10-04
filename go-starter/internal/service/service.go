// Package service implements the application capabilities declared by the domain
// package. It is shared by every transport.
package service

import (
	"context"
	"fmt"
	"sync/atomic"

	"github.com/codingseeker/go-starter/internal/buildinfo"
	"github.com/codingseeker/go-starter/internal/config"
	"github.com/codingseeker/go-starter/internal/domain"
)

// ApplicationService implements the application capabilities on top of the
// domain rules. It holds no transport specific state and is safe for concurrent
// use, so the CLI and the HTTP API can share a single instance.
type ApplicationService struct {
	cfg    config.Config
	closed atomic.Bool
}

// New builds the application service.
func New(cfg config.Config) *ApplicationService {
	return &ApplicationService{cfg: cfg}
}

// Health reports liveness. It only inspects in-process state and therefore
// never fails under normal operation.
func (s *ApplicationService) Health(_ context.Context) (domain.HealthStatus, error) {
	return domain.HealthStatus{
		Status:      domain.StatusOK,
		AppName:     s.cfg.AppName,
		Environment: s.cfg.Environment,
		Version:     buildinfo.Version,
		Checks: map[string]string{
			"config": "ok",
		},
	}, nil
}

// Ready reports whether the process can serve traffic. Readiness adds the
// shutdown state, so a draining instance is removed from load balancers before
// it stops accepting requests.
func (s *ApplicationService) Ready(ctx context.Context) (domain.HealthStatus, error) {
	status, err := s.Health(ctx)
	if err != nil {
		return status, fmt.Errorf("readiness depends on health: %w", err)
	}

	status.Checks["service"] = "ok"
	if s.closed.Load() {
		status.Status = domain.StatusDegraded
		status.Checks["service"] = "shutting down"
		return status, fmt.Errorf("service is shutting down")
	}
	return status, nil
}

// Greet is the demonstration operation shared by the CLI and the HTTP API.
func (s *ApplicationService) Greet(ctx context.Context, name string) (domain.Greeting, error) {
	if err := ctx.Err(); err != nil {
		return domain.Greeting{}, fmt.Errorf("greet aborted: %w", err)
	}

	clean, err := domain.NormalizeName(name)
	if err != nil {
		return domain.Greeting{}, fmt.Errorf("greet: %w", err)
	}

	return domain.Greeting{
		Name:    clean,
		Message: fmt.Sprintf("hello %s, %s says welcome to %s", clean, s.cfg.AppName, s.cfg.Environment),
	}, nil
}

// Close marks the service as draining. It is idempotent and lets readiness
// reflect the shutdown state.
func (s *ApplicationService) Close() error {
	s.closed.Store(true)
	return nil
}

var _ domain.Service = (*ApplicationService)(nil)
