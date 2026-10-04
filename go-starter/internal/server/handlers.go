package server

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"github.com/codingseeker/go-starter/internal/domain"
	"github.com/codingseeker/go-starter/internal/logger"
)

// API error codes. Clients match on these, never on the message text.
const (
	codeInvalidRequest = "invalid_request"
	codeInvalidJSON    = "invalid_json"
	codeNotFound       = "not_found"
	codeMethodNotAllow = "method_not_allowed"
	codeBodyTooLarge   = "body_too_large"
	codeUnsupported    = "unsupported_media_type"
	codeUnavailable    = "service_unavailable"
	codeInternal       = "internal_error"
)

// Generic client facing messages. Internal error details are logged, never
// returned, so stack traces and dependency messages stay internal.
const (
	messageInvalidRequest = "the request could not be processed"
	messageNotFound       = "resource not found"
	messageBodyTooLarge   = "request body exceeds the maximum allowed size"
	messageUnsupported    = "unsupported content type, expected application/json"
	messageUnavailable    = "service is not ready"
	messageInternal       = "internal server error"
)

// routes wires middleware and handlers. Every path also has a method agnostic
// registration so that a wrong method produces the same JSON error shape
// instead of the default plain text response of http.ServeMux.
func (s *Server) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", s.handleHealth)
	mux.HandleFunc("/health", s.handleMethodNotAllowed)
	mux.HandleFunc("GET /ready", s.handleReady)
	mux.HandleFunc("/ready", s.handleMethodNotAllowed)
	mux.HandleFunc("GET /v1/greet", s.handleGreet)
	mux.HandleFunc("/v1/greet", s.handleMethodNotAllowed)
	mux.HandleFunc("POST /v1/echo", s.handleEcho)
	mux.HandleFunc("/v1/echo", s.handleMethodNotAllowed)
	mux.HandleFunc("/", s.handleNotFound)

	// Order matters: the request id is attached first so every later stage can
	// log with it, the access log wraps the recoverer so that a panic is still
	// recorded with its 500 status, and the body limit sits closest to the
	// handlers so every read is capped.
	return chain(mux,
		s.requestID,
		s.accessLog,
		s.recoverer,
		s.securityHeaders,
		s.limitBody,
	)
}

// healthResponse is the payload of the health and readiness endpoints.
type healthResponse struct {
	Status      string            `json:"status"`
	App         string            `json:"app"`
	Environment string            `json:"environment"`
	Version     string            `json:"version"`
	Checks      map[string]string `json:"checks,omitempty"`
}

// greetResponse is the payload of the demonstration endpoint.
type greetResponse struct {
	Name    string `json:"name"`
	Message string `json:"message"`
}

// errorResponse is the single error shape returned by every endpoint.
type errorResponse struct {
	Error apiError `json:"error"`
}

// allowedMethods drives the Allow header of a 405 response.
var allowedMethods = map[string][]string{
	"/health":   {"GET", "HEAD"},
	"/ready":    {"GET", "HEAD"},
	"/v1/greet": {"GET"},
	"/v1/echo":  {"POST"},
}

type apiError struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	RequestID string `json:"request_id,omitempty"`
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	if s.service == nil {
		s.writeError(w, r, http.StatusServiceUnavailable, codeUnavailable, messageUnavailable)
		return
	}

	status, err := s.service.Health(r.Context())
	if err != nil {
		s.logInternal(r, "health check failed", err)
		s.writeError(w, r, http.StatusServiceUnavailable, codeUnavailable, messageUnavailable)
		return
	}
	s.writeJSON(w, r, http.StatusOK, healthResponse{
		Status:      status.Status,
		App:         status.AppName,
		Environment: status.Environment,
		Version:     status.Version,
		Checks:      status.Checks,
	})
}

func (s *Server) handleReady(w http.ResponseWriter, r *http.Request) {
	if s.service == nil {
		s.writeError(w, r, http.StatusServiceUnavailable, codeUnavailable, messageUnavailable)
		return
	}

	status, err := s.service.Ready(r.Context())
	if err != nil {
		s.logInternal(r, "readiness check failed", err)
		// The body still reports the reason so operators can debug, but no
		// internal error text is leaked.
		s.writeJSON(w, r, http.StatusServiceUnavailable, healthResponse{
			Status:      status.Status,
			App:         status.AppName,
			Environment: status.Environment,
			Version:     status.Version,
			Checks:      status.Checks,
		})
		return
	}
	s.writeJSON(w, r, http.StatusOK, healthResponse{
		Status:      status.Status,
		App:         status.AppName,
		Environment: status.Environment,
		Version:     status.Version,
		Checks:      status.Checks,
	})
}

func (s *Server) handleGreet(w http.ResponseWriter, r *http.Request) {
	if s.service == nil {
		s.writeError(w, r, http.StatusServiceUnavailable, codeUnavailable, messageUnavailable)
		return
	}

	name := r.URL.Query().Get("name")
	greeting, err := s.service.Greet(r.Context(), name)
	if err != nil {
		if errors.Is(err, domain.ErrInvalidName) {
			s.writeError(w, r, http.StatusBadRequest, codeInvalidRequest, err.Error())
			return
		}
		s.logInternal(r, "greet failed", err)
		s.writeError(w, r, http.StatusInternalServerError, codeInternal, messageInternal)
		return
	}

	s.writeJSON(w, r, http.StatusOK, greetResponse{Name: greeting.Name, Message: greeting.Message})
}

// echoRequest is intentionally permissive: the endpoint demonstrates body
// handling, not a schema library.
type echoRequest struct {
	Message string `json:"message"`
}

func (s *Server) handleEcho(w http.ResponseWriter, r *http.Request) {
	if ct := r.Header.Get("Content-Type"); ct != "" && !strings.HasPrefix(ct, "application/json") {
		s.writeError(w, r, http.StatusUnsupportedMediaType, codeUnsupported, messageUnsupported)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		if isBodyTooLarge(err) {
			s.writeError(w, r, http.StatusRequestEntityTooLarge, codeBodyTooLarge, messageBodyTooLarge)
			return
		}
		s.logInternal(r, "read request body failed", err)
		s.writeError(w, r, http.StatusBadRequest, codeInvalidRequest, messageInvalidRequest)
		return
	}

	if len(strings.TrimSpace(string(body))) == 0 {
		s.writeError(w, r, http.StatusBadRequest, codeInvalidJSON, "request body must not be empty")
		return
	}

	var request echoRequest
	if err := json.Unmarshal(body, &request); err != nil {
		if errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, io.EOF) {
			s.writeError(w, r, http.StatusBadRequest, codeInvalidJSON, "request body must be a single JSON object")
			return
		}
		s.writeError(w, r, http.StatusBadRequest, codeInvalidJSON, "request body must be valid JSON")
		return
	}
	if strings.TrimSpace(request.Message) == "" {
		s.writeError(w, r, http.StatusBadRequest, codeInvalidRequest, "field message must not be empty")
		return
	}

	// Proves context propagation: the request scoped logger carries the
	// request id that the middleware stored in the context.
	logger.FromContext(r.Context()).DebugContext(r.Context(), "echo accepted",
		slog.Int("message_length", len(request.Message)))

	s.writeJSON(w, r, http.StatusOK, greetResponse{Name: "echo", Message: request.Message})
}

func (s *Server) handleNotFound(w http.ResponseWriter, r *http.Request) {
	s.writeError(w, r, http.StatusNotFound, codeNotFound, messageNotFound)
}

func (s *Server) handleMethodNotAllowed(w http.ResponseWriter, r *http.Request) {
	allowed := make([]string, 0, 2)
	if methods, ok := allowedMethods[r.URL.Path]; ok {
		allowed = methods
	}
	w.Header().Set("Allow", strings.Join(allowed, ", "))
	s.writeError(w, r, http.StatusMethodNotAllowed, codeMethodNotAllow, "method "+r.Method+" is not allowed on this resource")
}

// writeJSON is the single place that serialises successful responses.
func (s *Server) writeJSON(w http.ResponseWriter, r *http.Request, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)

	if err := json.NewEncoder(w).Encode(payload); err != nil {
		s.logInternal(r, "encode response failed", err)
	}
}

// writeError is the centralised error writer. Callers pass a stable code and a
// safe message; anything sensitive is logged instead.
func (s *Server) writeError(w http.ResponseWriter, r *http.Request, status int, code, message string) {
	s.writeJSON(w, r, status, errorResponse{Error: apiError{
		Code:      code,
		Message:   message,
		RequestID: requestIDFromContext(r.Context()),
	}})
}

func (s *Server) logInternal(r *http.Request, message string, err error) {
	logger.FromContext(r.Context()).ErrorContext(r.Context(), message, slog.String("error", err.Error()))
}

// newRequestID returns an unguessable identifier for correlation.
func newRequestID() string {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return ""
	}
	return hex.EncodeToString(buf)
}
