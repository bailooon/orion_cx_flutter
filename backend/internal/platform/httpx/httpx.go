// Package httpx holds the HTTP plumbing shared by all Orion services: a single
// JSON error envelope, request middleware, and a service-to-service client
// with timeouts and retries (RNF001, RNF007).
package httpx

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net"
	"net/http"
	"time"

	"github.com/google/uuid"
)

type ctxKey string

const requestIDKey ctxKey = "orion.request_id"

// ErrorBody is the single error shape every service returns, so the Flutter
// client only has to understand one contract.
type ErrorBody struct {
	Error   string `json:"error"`
	Message string `json:"message"`
	Detail  string `json:"detail,omitempty"`
}

// APIError is an error carrying the HTTP status it should be rendered with.
type APIError struct {
	Status  int
	Code    string
	Message string
}

func (e APIError) Error() string { return fmt.Sprintf("%s: %s", e.Code, e.Message) }

// Common errors reused across services.
var (
	ErrNotFound     = APIError{Status: http.StatusNotFound, Code: "not_found", Message: "Recurso não encontrado."}
	ErrUnauthorized = APIError{Status: http.StatusUnauthorized, Code: "unauthorized", Message: "Credenciais inválidas ou expiradas."}
	ErrForbidden    = APIError{Status: http.StatusForbidden, Code: "forbidden", Message: "Acesso negado para este perfil."}
)

// BadRequest builds a 400 with a caller-supplied message.
func BadRequest(message string) APIError {
	return APIError{Status: http.StatusBadRequest, Code: "bad_request", Message: message}
}

// Conflict builds a 409 with a caller-supplied message.
func Conflict(message string) APIError {
	return APIError{Status: http.StatusConflict, Code: "conflict", Message: message}
}

// WriteJSON writes v as JSON with the given status.
func WriteJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if v == nil {
		return
	}
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Error("failed to encode response", slog.String("err", err.Error()))
	}
}

// WriteError renders err using the API error envelope. Unknown errors become a
// generic 500 so internal details never reach the client.
func WriteError(w http.ResponseWriter, err error) {
	var apiErr APIError
	if !errors.As(err, &apiErr) {
		apiErr = APIError{
			Status:  http.StatusInternalServerError,
			Code:    "internal_error",
			Message: "Não foi possível concluir a operação. Tente novamente.",
		}
	}
	WriteJSON(w, apiErr.Status, ErrorBody{Error: apiErr.Code, Message: apiErr.Message})
}

// DecodeJSON reads a JSON body with a size limit and returns a 400 on failure.
func DecodeJSON(r *http.Request, target any) error {
	defer func() { _, _ = io.Copy(io.Discard, r.Body) }()
	limited := http.MaxBytesReader(nil, r.Body, 1<<20)
	if err := json.NewDecoder(limited).Decode(target); err != nil {
		return BadRequest("Corpo da requisição inválido.")
	}
	return nil
}

// RequestID returns the correlation id attached to the request context.
func RequestID(ctx context.Context) string {
	if value, ok := ctx.Value(requestIDKey).(string); ok {
		return value
	}
	return ""
}

// WithRequestID stamps a correlation id on every request and echoes it back, so
// one customer interaction can be traced across the five services.
func WithRequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get("X-Request-Id")
		if id == "" {
			id = uuid.NewString()
		}
		w.Header().Set("X-Request-Id", id)
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), requestIDKey, id)))
	})
}

// statusRecorder captures the response status for the access log.
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(status int) {
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}

// Hijack lets the WebSocket upgrade work through the logging middleware.
func (r *statusRecorder) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	hijacker, ok := r.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, errors.New("response writer does not support hijacking")
	}
	return hijacker.Hijack()
}

// Flush forwards flushes to the underlying writer.
func (r *statusRecorder) Flush() {
	if flusher, ok := r.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

// AccessLog logs method, path, status and latency. Query strings and bodies are
// deliberately not logged: they can carry personal data.
func AccessLog(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			started := time.Now()
			recorder := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
			next.ServeHTTP(recorder, r)
			logger.Info("http",
				slog.String("method", r.Method),
				slog.String("path", r.URL.Path),
				slog.Int("status", recorder.status),
				slog.Int64("duration_ms", time.Since(started).Milliseconds()),
				slog.String("request_id", RequestID(r.Context())),
			)
		})
	}
}

// Recover turns a panic into a 500 instead of killing the process (RNF002).
func Recover(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if recovered := recover(); recovered != nil {
					logger.Error("panic recovered",
						slog.Any("panic", recovered),
						slog.String("path", r.URL.Path),
						slog.String("request_id", RequestID(r.Context())),
					)
					WriteError(w, errors.New("panic"))
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}

// Client is the service-to-service HTTP client. Every call is bounded by a
// timeout and retried with exponential backoff on transient failures, so a
// restarting dependency degrades the response instead of hanging it.
type Client struct {
	base    string
	http    *http.Client
	retries int
	logger  *slog.Logger
}

// NewClient builds a client pointing at baseURL.
func NewClient(baseURL string, timeout time.Duration, retries int, logger *slog.Logger) *Client {
	return &Client{
		base:    baseURL,
		http:    &http.Client{Timeout: timeout},
		retries: retries,
		logger:  logger,
	}
}

// Do issues a JSON request against the peer service and decodes the response
// into out, which may be nil. Retries cover connection errors and 5xx only:
// a 4xx is a deterministic answer and is surfaced straight to the caller.
func (c *Client) Do(ctx context.Context, method, path string, in, out any) error {
	var payload []byte
	if in != nil {
		encoded, err := json.Marshal(in)
		if err != nil {
			return fmt.Errorf("encode request: %w", err)
		}
		payload = encoded
	}

	var lastErr error
	for attempt := 0; attempt <= c.retries; attempt++ {
		if attempt > 0 {
			delay := time.Duration(math.Pow(2, float64(attempt-1))) * 50 * time.Millisecond
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(delay):
			}
		}

		req, err := http.NewRequestWithContext(ctx, method, c.base+path, bytes.NewReader(payload))
		if err != nil {
			return fmt.Errorf("build request: %w", err)
		}
		req.Header.Set("Content-Type", "application/json")
		if id := RequestID(ctx); id != "" {
			req.Header.Set("X-Request-Id", id)
		}

		resp, err := c.http.Do(req)
		if err != nil {
			lastErr = err
			continue
		}

		body, readErr := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		_ = resp.Body.Close()
		if readErr != nil {
			lastErr = readErr
			continue
		}

		if resp.StatusCode >= 500 {
			lastErr = fmt.Errorf("upstream %s%s returned %d", c.base, path, resp.StatusCode)
			continue
		}
		if resp.StatusCode >= 400 {
			var decoded ErrorBody
			_ = json.Unmarshal(body, &decoded)
			message := decoded.Message
			if message == "" {
				message = "Falha na chamada interna."
			}
			return APIError{Status: resp.StatusCode, Code: decoded.Error, Message: message}
		}
		if out != nil && len(body) > 0 {
			if err := json.Unmarshal(body, out); err != nil {
				return fmt.Errorf("decode response: %w", err)
			}
		}
		return nil
	}

	c.logger.Warn("internal call exhausted retries",
		slog.String("target", c.base+path),
		slog.String("err", lastErr.Error()),
	)
	return fmt.Errorf("call %s%s: %w", c.base, path, lastErr)
}
