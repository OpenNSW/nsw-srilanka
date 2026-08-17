package tasks

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	nswaudit "github.com/OpenNSW/nsw-srilanka/internal/audit"
)

// mockAuditRecorder implements nswaudit.Auditor and collects events for assertions.
type mockAuditRecorder struct {
	events []nswaudit.Event
}

func (m *mockAuditRecorder) Audit(_ context.Context, e nswaudit.Event) {
	m.events = append(m.events, e)
}

func TestNewHTTPHandler_SetsMaxRequestBytes(t *testing.T) {
	for _, v := range []int64{1024, 0, -1, -33554432} {
		handler := NewHTTPHandler(nil, nil, nil, v, nil)
		if handler.MaxRequestBytes != v {
			t.Errorf("MaxRequestBytes = %d, want %d", handler.MaxRequestBytes, v)
		}
	}
}

func TestHandleCompleteTaskStep_AuditParseFailure(t *testing.T) {
	auditor := &mockAuditRecorder{}
	handler := NewHTTPHandler(nil, nil, nil, 1024, auditor)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/tasks/task-123/commands/approve", strings.NewReader(`{invalid`))
	req.SetPathValue("id", "task-123")
	req.SetPathValue("command", "approve")
	rec := httptest.NewRecorder()

	handler.HandleCompleteTaskStep(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Len(t, auditor.events, 1)
	event := auditor.events[0]
	assert.Equal(t, nswaudit.EventTask, event.EventType)
	assert.True(t, event.Failure)
	assert.Equal(t, "task-123", event.TargetID)
	assert.Equal(t, "approve", event.Metadata["command"])
	assert.Equal(t, http.StatusBadRequest, event.Metadata["status"])
}

func TestHandleCompleteTaskStep_RejectsOversizedBody(t *testing.T) {
	handler := &HTTPHandler{MaxRequestBytes: 8}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/tasks/123", strings.NewReader(`{"command":"approve","payload":{"key":"value"}}`))
	req.SetPathValue("id", "123")
	recorder := httptest.NewRecorder()

	handler.HandleCompleteTaskStep(recorder, req)

	if recorder.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("expected 413, got %d", recorder.Code)
	}
	if !strings.Contains(recorder.Body.String(), errRequestBodyTooLarge) {
		t.Fatalf("expected error body to mention %q, got %s", errRequestBodyTooLarge, recorder.Body.String())
	}
}

func TestHandleCompleteTaskStep_RejectsTrailingDataAfterJSON(t *testing.T) {
	handler := &HTTPHandler{MaxRequestBytes: 1024}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/tasks/123", strings.NewReader(`{"command":"approve","payload":{"key":"value"}}{"command":"escalate"}`))
	req.SetPathValue("id", "123")
	recorder := httptest.NewRecorder()

	handler.HandleCompleteTaskStep(recorder, req)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", recorder.Code)
	}
	if !strings.Contains(recorder.Body.String(), errInvalidRequestBody) {
		t.Fatalf("expected error body to mention %q, got %s", errInvalidRequestBody, recorder.Body.String())
	}
}

func TestParseCompleteTaskStepRequest_AllowsTrailingWhitespace(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/api/v1/tasks/123/commands/approve", strings.NewReader(`{"key":"value"}`+"\n"))

	command, payload, _, _, err := parseCompleteTaskStepRequest(req, "approve")
	if err != nil {
		t.Fatalf("unexpected error for trailing whitespace: %v", err)
	}
	if command != "approve" {
		t.Fatalf("command = %q, want %q", command, "approve")
	}
	if payload["key"] != "value" {
		t.Fatalf("payload[\"key\"] = %v, want %q", payload["key"], "value")
	}
}
