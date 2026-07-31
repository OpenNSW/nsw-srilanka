package tasks

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNewHTTPHandler_RejectsNonPositiveMaxRequestBytes(t *testing.T) {
	for _, v := range []int64{0, -1, -33554432} {
		if _, err := NewHTTPHandler(nil, nil, nil, v); err == nil {
			t.Errorf("expected error for maxRequestBytes=%d, got nil", v)
		}
	}
}

func TestNewHTTPHandler_AcceptsPositiveMaxRequestBytes(t *testing.T) {
	handler, err := NewHTTPHandler(nil, nil, nil, 1024)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if handler.MaxRequestBytes != 1024 {
		t.Errorf("MaxRequestBytes = %d, want 1024", handler.MaxRequestBytes)
	}
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

	command, payload, _, err := parseCompleteTaskStepRequest(req, "approve")
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
