package tasks

import (
	"context"
	"crypto"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	argus "github.com/LSFLK/argus/pkg/audit"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	nswaudit "github.com/OpenNSW/nsw-srilanka/internal/audit"
)

type mockAuditor struct {
	mu     sync.Mutex
	events []*argus.AuditLogRequest
}

func (m *mockAuditor) LogEvent(_ context.Context, event *argus.AuditLogRequest) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.events = append(m.events, event)
	return true
}

func (m *mockAuditor) IsEnabled() bool { return true }

func (m *mockAuditor) SignEvent(context.Context, *argus.AuditLogRequest) error { return nil }

func (m *mockAuditor) SignMessageBytes(context.Context, []byte) (string, error) { return "", nil }

func (m *mockAuditor) LogSignedEvent(context.Context, *argus.AuditLogRequest) {}

func (m *mockAuditor) VerifyIntegrity(*argus.AuditLogRequest, crypto.PublicKey) (bool, error) {
	return true, nil
}

func (m *mockAuditor) Close(context.Context) error { return nil }

func (m *mockAuditor) eventsCopy() []*argus.AuditLogRequest {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]*argus.AuditLogRequest, len(m.events))
	copy(out, m.events)
	return out
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
	auditor := &mockAuditor{}
	recorder := nswaudit.NewRecorder(auditor)
	handler := NewHTTPHandler(nil, nil, nil, 1024, recorder)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/tasks/task-123/commands/approve", strings.NewReader(`{invalid`))
	req.SetPathValue("id", "task-123")
	req.SetPathValue("command", "approve")
	rec := httptest.NewRecorder()

	handler.HandleCompleteTaskStep(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code)
	events := auditor.eventsCopy()
	require.Len(t, events, 1)
	event := events[0]
	assert.Equal(t, string(nswaudit.EventTask), event.EventType)
	assert.Equal(t, argus.StatusFailure, event.Status)
	require.NotNil(t, event.TargetID)
	assert.Equal(t, "task-123", *event.TargetID)
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
