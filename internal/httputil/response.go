// Package httputil provides shared HTTP response helpers so API error.
package httputil

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/OpenNSW/core/trace"
)

// ErrorResponse is the standard JSON shape for API error bodies.
type ErrorResponse struct {
	Error         string `json:"error"`
	CorrelationID string `json:"correlationId,omitempty"`
}

// JSON writes payload as the JSON response body with the given status.
func JSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		slog.Error("httputil: failed to encode JSON response", "error", err)
	}
}

// TraceID returns the request's correlation ID from the trace middleware —
// the same value included in the correlationId field of Error/
// InternalServerError responses. Error itself does not log (see its doc
// comment); call sites that log before calling Error should attach this as a
// "traceId" attribute so the correlationId returned to the client is
// actually searchable in server logs.
func TraceID(r *http.Request) string {
	return trace.GetTraceID(r.Context())
}

// Error responds with a fixed, safe message for an expected client-facing condition.
func Error(w http.ResponseWriter, r *http.Request, status int, message string) {
	JSON(w, status, ErrorResponse{Error: message, CorrelationID: TraceID(r)})
}

// InternalServerError logs err server-side and responds with a generic, safe message to the client.
func InternalServerError(w http.ResponseWriter, r *http.Request, logMessage string, err error, attrs ...any) {
	traceID := TraceID(r)
	args := append([]any{"error", err, "traceId", traceID}, attrs...)
	slog.ErrorContext(r.Context(), logMessage, args...)
	JSON(w, http.StatusInternalServerError, ErrorResponse{
		Error:         "An error occurred while processing your request",
		CorrelationID: traceID,
	})
}
