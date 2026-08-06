package bootstrap

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/OpenNSW/core/httputil"
	"github.com/OpenNSW/core/trace"
	"github.com/OpenNSW/nsw-srilanka/cmd/server/config"
)

func TestErrorResponseCorrelationIDMatchesTraceID(t *testing.T) {
	// The correlationId returned to a client must equal the traceId in server
	// logs, or a client-reported ID can't be looked up. See PR #276.
	ConfigureLogging(&config.Config{Server: config.ServerConfig{LogLevel: slog.LevelInfo}})

	var got httputil.ErrorResponse
	var traceID string

	h := trace.TraceMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		traceID = trace.GetTraceID(r.Context())
		httputil.Error(w, r, http.StatusNotFound, "not found")
	}))

	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/test", nil))

	if err := json.NewDecoder(w.Body).Decode(&got); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if traceID == "" {
		t.Fatal("middleware did not set a trace ID")
	}
	if got.CorrelationID != traceID {
		t.Fatalf("correlationId %q does not match traceId %q", got.CorrelationID, traceID)
	}
}
