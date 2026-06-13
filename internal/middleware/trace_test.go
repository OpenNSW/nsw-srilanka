package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/OpenNSW/nsw-srilanka/internal/audit"
)

func TestTraceMiddleware(t *testing.T) {
	handler := TraceMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		traceID := r.Context().Value(audit.TraceIDKey)
		if traceID == nil {
			t.Errorf("Expected trace ID to be present in context, got nil")
			return
		}
		if traceID.(string) != "test-trace-123" {
			t.Errorf("Expected trace ID 'test-trace-123', got %s", traceID)
		}
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/api/v1/test", nil)
	req.Header.Set("X-Trace-ID", "test-trace-123")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Header().Get("X-Trace-ID") != "test-trace-123" {
		t.Errorf("Expected X-Trace-ID header in response to be 'test-trace-123', got %s", w.Header().Get("X-Trace-ID"))
	}
}
