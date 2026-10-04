package server

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"runtime/debug"
	"strings"
	"time"

	"github.com/codingseeker/go-starter/internal/logger"
)

// requestIDHeader carries the correlation identifier in and out of the process.
const requestIDHeader = "X-Request-Id"

// requestIDKey stores the correlation identifier in the request context.
type requestIDKey struct{}

// requestIDFromContext returns the correlation identifier of the request.
func requestIDFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	id, _ := ctx.Value(requestIDKey{}).(string)
	return id
}

// statusRecorder captures the response status for access logging.
type statusRecorder struct {
	http.ResponseWriter
	status int
	bytes  int
}

func (w *statusRecorder) WriteHeader(status int) {
	if w.status == 0 {
		w.status = status
	}
	w.ResponseWriter.WriteHeader(status)
}

func (w *statusRecorder) Write(b []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	n, err := w.ResponseWriter.Write(b)
	w.bytes += n
	return n, err
}

// Unwrap exposes the underlying writer to http.ResponseController.
func (w *statusRecorder) Unwrap() http.ResponseWriter { return w.ResponseWriter }

// chain applies middleware in the given order, so the first entry is the
// outermost handler.
func chain(h http.Handler, middleware ...func(http.Handler) http.Handler) http.Handler {
	for i := len(middleware) - 1; i >= 0; i-- {
		h = middleware[i](h)
	}
	return h
}

// recoverer converts a panic into a 500 response and keeps the process alive.
// The stack trace stays in the logs, never in the response.
func (s *Server) recoverer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if recovered := recover(); recovered != nil {
				log := logger.FromContextOr(r.Context(), s.logger)
				log.ErrorContext(r.Context(), "panic while handling request",
					slog.Any("panic", recovered),
					slog.String("stack", string(debug.Stack())),
				)
				s.writeError(w, r, http.StatusInternalServerError, codeInternal, messageInternal)
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// requestID propagates an existing correlation id or generates one, and stores
// a request scoped logger in the request context.
func (s *Server) requestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get(requestIDHeader)
		if id == "" {
			id = newRequestID()
		}
		w.Header().Set(requestIDHeader, id)

		log := s.logger.With(
			slog.String("request_id", id),
			slog.String("method", r.Method),
			slog.String("path", r.URL.Path),
		)
		ctx := context.WithValue(r.Context(), requestIDKey{}, id)
		ctx = logger.WithContext(ctx, log)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// accessLog records one structured line per request. The deferred write
// guarantees a line even when a handler panics.
func (s *Server) accessLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		recorder := &statusRecorder{ResponseWriter: w}

		defer func() {
			status := recorder.status
			if status == 0 {
				status = http.StatusOK
			}
			logger.FromContextOr(r.Context(), s.logger).InfoContext(r.Context(), "http request",
				slog.Int("status", status),
				slog.Int("bytes", recorder.bytes),
				slog.String("remote_addr", clientIP(r)),
				slog.Duration("duration", time.Since(start)),
			)
		}()

		next.ServeHTTP(recorder, r)
	})
}

// limitBody caps the number of bytes a handler may read from the body.
func (s *Server) limitBody(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Body != nil {
			r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodySize)
		}
		next.ServeHTTP(w, r)
	})
}

// securityHeaders applies conservative defaults for an API that is consumed by
// machines rather than browsers.
func (s *Server) securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("Cache-Control", "no-store")
		next.ServeHTTP(w, r)
	})
}

func clientIP(r *http.Request) string {
	if forwarded := r.Header.Get("X-Forwarded-For"); forwarded != "" {
		return firstIP(forwarded)
	}
	if real := r.Header.Get("X-Real-Ip"); real != "" {
		return real
	}
	return r.RemoteAddr
}

// firstIP extracts the originating client from a X-Forwarded-For list.
func firstIP(header string) string {
	if idx := strings.IndexByte(header, ','); idx >= 0 {
		return strings.TrimSpace(header[:idx])
	}
	return strings.TrimSpace(header)
}

// isBodyTooLarge reports whether err was produced by http.MaxBytesReader.
func isBodyTooLarge(err error) bool {
	var tooLarge *http.MaxBytesError
	return errors.As(err, &tooLarge)
}
