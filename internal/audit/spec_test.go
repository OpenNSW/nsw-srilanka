package audit

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	argus "github.com/LSFLK/argus/pkg/audit"
)

func TestSpec_Wrap_PathID(t *testing.T) {
	client := &mockAuditor{enabled: true}
	recorder := NewRecorder(client)

	spec := Spec{
		EventType:        EventConsignment,
		TargetType:       TargetConsignment,
		TargetIDFromPath: "id",
	}

	handler := recorder.Wrap(spec)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"success"}`))
	}))

	req := httptest.NewRequest("DELETE", "/api/v1/consignments/con-abc", nil)
	req.SetPathValue("id", "con-abc") // simulate routing path capture
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	events := client.getEvents()
	if len(events) != 1 {
		t.Fatalf("expected 1 logged event, got %d", len(events))
	}

	ev := events[0]
	if ev.Action != ActionDelete {
		t.Errorf("expected Action %s, got %s", ActionDelete, ev.Action)
	}
	if ev.TargetID == nil || *ev.TargetID != "con-abc" {
		t.Errorf("expected TargetID con-abc, got %v", ev.TargetID)
	}
	if ev.Status != argus.StatusSuccess {
		t.Errorf("expected Status SUCCESS, got %s", ev.Status)
	}
}

func TestSpec_Wrap_RespBodyID(t *testing.T) {
	client := &mockAuditor{enabled: true}
	recorder := NewRecorder(client)

	spec := Spec{
		EventType:        EventStorage,
		TargetType:       TargetStorage,
		TargetIDFromResp: "key",
	}

	handler := recorder.Wrap(spec)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"key":"storage/foo.pdf","size":123}`))
	}))

	req := httptest.NewRequest("POST", "/api/v1/storage", bytes.NewBuffer([]byte(`{"file":"foo.pdf"}`)))
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	events := client.getEvents()
	if len(events) != 1 {
		t.Fatalf("expected 1 logged event, got %d", len(events))
	}

	ev := events[0]
	if ev.Action != ActionCreate {
		t.Errorf("expected Action %s, got %s", ActionCreate, ev.Action)
	}
	if ev.TargetID == nil || *ev.TargetID != "storage/foo.pdf" {
		t.Errorf("expected TargetID storage/foo.pdf, got %v", ev.TargetID)
	}
	if ev.Status != argus.StatusSuccess {
		t.Errorf("expected Status SUCCESS, got %s", ev.Status)
	}
}

func TestSpec_Wrap_FailureStatus(t *testing.T) {
	client := &mockAuditor{enabled: true}
	recorder := NewRecorder(client)

	spec := Spec{
		EventType:        EventTask,
		TargetType:       TargetTask,
		TargetIDFromPath: "id",
	}

	handler := recorder.Wrap(spec)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"bad request"}`))
	}))

	req := httptest.NewRequest("POST", "/api/v1/tasks/task-99", nil)
	req.SetPathValue("id", "task-99")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	events := client.getEvents()
	if len(events) != 1 {
		t.Fatalf("expected 1 logged event, got %d", len(events))
	}

	ev := events[0]
	if ev.Status != argus.StatusFailure {
		t.Errorf("expected Status FAILURE, got %s", ev.Status)
	}
}

func TestSpec_Wrap_RespBodyNumericID(t *testing.T) {
	client := &mockAuditor{enabled: true}
	recorder := NewRecorder(client)

	spec := Spec{
		EventType:        EventConsignment,
		TargetType:       TargetConsignment,
		TargetIDFromResp: "id",
	}

	handler := recorder.Wrap(spec)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":12345,"status":"draft"}`))
	}))

	req := httptest.NewRequest("POST", "/api/v1/consignments", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	events := client.getEvents()
	if len(events) != 1 {
		t.Fatalf("expected 1 logged event, got %d", len(events))
	}

	ev := events[0]
	if ev.TargetID == nil || *ev.TargetID != "12345" {
		t.Errorf("expected TargetID 12345, got %v", ev.TargetID)
	}
}
