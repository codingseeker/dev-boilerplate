// Package server exposes the application services over HTTP. It owns routing,
// middleware, request validation and the HTTP lifecycle only.
package server

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"sync"

	"github.com/codingseeker/go-starter/internal/config"
	"github.com/codingseeker/go-starter/internal/domain"
	"github.com/codingseeker/go-starter/internal/logger"
)

const (
	// maxRequestBodySize caps request bodies accepted by the API.
	maxRequestBodySize = 1 << 20 // 1 MiB
	// maxHeaderBytes caps request headers to limit header based attacks.
	maxHeaderBytes = 1 << 20 // 1 MiB
)

// Server exposes the application services over HTTP. Routing, middleware and
// transport concerns live here; business logic stays in the injected service.
type Server struct {
	config  config.Config
	logger  *slog.Logger
	service domain.Service

	handler http.Handler

	mu       sync.Mutex
	httpSrv  *http.Server
	listener net.Listener
	addr     string
}

// New builds an HTTP server. A nil logger falls back to a no-op logger so the
// server is usable in tests without global state.
func New(cfg config.Config, log *slog.Logger, svc domain.Service) *Server {
	if log == nil {
		log = logger.Noop()
	}
	s := &Server{
		config:  cfg,
		logger:  log,
		service: svc,
	}
	s.handler = s.routes()
	return s
}

// Handler exposes the fully decorated HTTP handler, which makes the API
// testable with httptest without binding a socket.
func (s *Server) Handler() http.Handler { return s.handler }

// Addr reports the address the server listens on. It is only meaningful after
// Start has bound the listener; with port 0 it resolves the port chosen by the
// operating system.
func (s *Server) Addr() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.addr
}

// Start binds the listener and serves until Shutdown is called or the context
// is cancelled.
func (s *Server) Start(ctx context.Context) error {
	s.mu.Lock()
	if s.httpSrv != nil {
		s.mu.Unlock()
		return errors.New("server is already started")
	}

	listener, err := (&net.ListenConfig{}).Listen(ctx, "tcp", s.config.HTTP.Addr())
	if err != nil {
		s.mu.Unlock()
		return fmt.Errorf("listen on %s: %w", s.config.HTTP.Addr(), err)
	}

	httpSrv := s.newHTTPServer()
	s.httpSrv = httpSrv
	s.listener = listener
	s.addr = listener.Addr().String()
	s.mu.Unlock()

	s.logger.InfoContext(ctx, "http server listening", slog.String("address", s.Addr()))

	if err := httpSrv.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("serve http: %w", err)
	}
	return nil
}

// newHTTPServer applies the configured timeouts and the safe defaults that do
// not depend on configuration.
func (s *Server) newHTTPServer() *http.Server {
	return &http.Server{
		Addr:              s.config.HTTP.Addr(),
		Handler:           s.handler,
		ReadHeaderTimeout: s.config.HTTP.ReadTimeout,
		ReadTimeout:       s.config.HTTP.ReadTimeout,
		WriteTimeout:      s.config.HTTP.WriteTimeout,
		IdleTimeout:       s.config.HTTP.IdleTimeout,
		MaxHeaderBytes:    maxHeaderBytes,
		// Route the stdlib server errors through the structured logger.
		ErrorLog: slog.NewLogLogger(s.logger.Handler(), slog.LevelWarn),
	}
}

// withDrainDeadline guarantees a bounded drain. A caller supplied deadline
// always wins, so the application layer controls the overall shutdown budget.
func (s *Server) withDrainDeadline(ctx context.Context) (context.Context, context.CancelFunc) {
	if _, ok := ctx.Deadline(); ok {
		return context.WithCancel(ctx)
	}
	timeout := s.config.HTTP.ShutdownTimeout
	if timeout <= 0 {
		timeout = config.DefaultShutdownTimeout
	}
	return context.WithTimeout(ctx, timeout)
}

// Shutdown drains in-flight requests, bounded by the configured timeout, and
// force closes the listener if the deadline expires. It is safe to call before
// Start and safe to call twice.
func (s *Server) Shutdown(ctx context.Context) error {
	s.mu.Lock()
	httpSrv := s.httpSrv
	addr := s.addr
	s.httpSrv = nil
	s.listener = nil
	s.addr = ""
	s.mu.Unlock()

	if httpSrv == nil {
		return nil
	}

	s.logger.InfoContext(ctx, "http server draining",
		slog.String("address", addr),
		slog.Duration("timeout", s.config.HTTP.ShutdownTimeout),
	)

	drainCtx, cancel := s.withDrainDeadline(ctx)
	defer cancel()

	if err := httpSrv.Shutdown(drainCtx); err != nil {
		s.logger.WarnContext(ctx, "graceful shutdown failed, closing connections",
			slog.String("error", err.Error()))
		if closeErr := httpSrv.Close(); closeErr != nil {
			return errors.Join(fmt.Errorf("graceful shutdown failed: %w", err), fmt.Errorf("force close failed: %w", closeErr))
		}
		return fmt.Errorf("graceful shutdown failed: %w", err)
	}

	s.logger.InfoContext(ctx, "http server stopped")
	return nil
}
