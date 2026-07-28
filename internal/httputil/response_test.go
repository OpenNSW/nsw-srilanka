package httputil

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/OpenNSW/core/trace"
)

func TestJSON_WritesStatusContentTypeAndBody(t *testing.T) {
	w := httptest.NewRecorder()

	JSON(w, http.StatusCreated, map[string]string{"id": "c-1"})

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/json" {
		t.Fatalf("expected application/json, got %q", ct)
	}
	var got map[string]string
	if err := json.NewDecoder(w.Body).Decode(&got); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if got["id"] != "c-1" {
		t.Fatalf("unexpected body: %+v", got)
	}
}

type errorWriter struct{ header http.Header }

func (e *errorWriter) Header() http.Header       { return e.header }
func (e *errorWriter) Write([]byte) (int, error) { return 0, errors.New("write error") }
func (e *errorWriter) WriteHeader(int)           {}

func TestJSON_EncodeFailureDoesNotPanic(t *testing.T) {
	JSON(&errorWriter{header: http.Header{}}, http.StatusOK, map[string]string{"id": "c-1"})
}

func TestTraceID_MatchesCorrelationIDSentToClient(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/companies", nil)
	req = req.WithContext(trace.ContextWithTraceID(req.Context(), "trace-xyz"))

	if got := TraceID(req); got != "trace-xyz" {
		t.Fatalf("expected TraceID %q, got %q", "trace-xyz", got)
	}

	// TraceID must return the same value callers attach to their own log
	// lines as the one Error/InternalServerError send back as correlationId
	// — otherwise a client-reported correlationId isn't searchable in logs.
	w := httptest.NewRecorder()
	Error(w, req, http.StatusNotFound, "company not found")
	var got ErrorResponse
	if err := json.NewDecoder(w.Body).Decode(&got); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if got.CorrelationID != TraceID(req) {
		t.Fatalf("correlationId %q does not match TraceID(r) %q", got.CorrelationID, TraceID(req))
	}
}

func TestTraceID_EmptyWhenNoneSet(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/companies", nil)

	if got := TraceID(req); got != "" {
		t.Fatalf("expected empty TraceID, got %q", got)
	}
}

func TestError_WritesMessageAndCorrelationID(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/companies", nil)
	req = req.WithContext(trace.ContextWithTraceID(req.Context(), "trace-123"))
	w := httptest.NewRecorder()

	Error(w, req, http.StatusNotFound, "company not found")

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
	var got ErrorResponse
	if err := json.NewDecoder(w.Body).Decode(&got); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if got.Error != "company not found" {
		t.Fatalf("expected message %q, got %q", "company not found", got.Error)
	}
	if got.CorrelationID != "trace-123" {
		t.Fatalf("expected correlationId %q, got %q", "trace-123", got.CorrelationID)
	}
}

func TestError_NoTraceIDOmitsCorrelationID(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/companies", nil)
	w := httptest.NewRecorder()

	Error(w, req, http.StatusBadRequest, "invalid pagination parameters")

	body := w.Body.String()
	if strings.Contains(body, "correlationId") {
		t.Fatalf("expected correlationId to be omitted when no trace ID is set, got body: %s", body)
	}
	var got ErrorResponse
	if err := json.NewDecoder(strings.NewReader(body)).Decode(&got); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if got.Error != "invalid pagination parameters" {
		t.Fatalf("unexpected error message: %q", got.Error)
	}
}

func TestInternalServerError_NeverExposesUnderlyingErrorText(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/consignments", nil)
	req = req.WithContext(trace.ContextWithTraceID(req.Context(), "trace-abc"))
	w := httptest.NewRecorder()

	sensitiveErr := errors.New("pq: relation \"consignments\" does not exist")
	InternalServerError(w, req, "failed to retrieve consignments", sensitiveErr)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", w.Code)
	}
	var got ErrorResponse
	if err := json.NewDecoder(w.Body).Decode(&got); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if strings.Contains(got.Error, "pq:") || strings.Contains(got.Error, "relation") {
		t.Fatalf("response leaked underlying error text: %q", got.Error)
	}
	if got.Error != "An error occurred while processing your request" {
		t.Fatalf("unexpected generic message: %q", got.Error)
	}
	if got.CorrelationID != "trace-abc" {
		t.Fatalf("expected correlationId %q, got %q", "trace-abc", got.CorrelationID)
	}
}

func TestInternalServerError_NilErrDoesNotPanic(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/consignments", nil)
	w := httptest.NewRecorder()

	InternalServerError(w, req, "consignment is nil after successful creation", nil)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", w.Code)
	}
}

func TestInternalServerError_WithAttrsDoesNotPanic(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/consignments", nil)
	w := httptest.NewRecorder()

	InternalServerError(w, req, "failed to resolve user company", errors.New("boom"), "ouHandle", "acme-corp")

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", w.Code)
	}
}
