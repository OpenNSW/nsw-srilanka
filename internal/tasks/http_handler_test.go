package tasks

import (
	"context"
	"crypto"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	argus "github.com/LSFLK/argus/pkg/audit"
	"github.com/OpenNSW/core/taskflow/renderer/zoneview"
	"github.com/OpenNSW/core/taskflow/store"
	"github.com/OpenNSW/core/uiprojector"
	nswaudit "github.com/OpenNSW/nsw-srilanka/internal/audit"
	"github.com/OpenNSW/nsw-srilanka/internal/tasks/readauthz"
	"github.com/OpenNSW/nsw-srilanka/internal/tasks/taskauthz"
)

func TestNewHTTPHandler_SetsMaxRequestBytes(t *testing.T) {
	for _, v := range []int64{1024, 0, -1, -33554432} {
		handler := NewHTTPHandler(nil, nil, nil, nil, nil, v)
		if handler.MaxRequestBytes != v {
			t.Errorf("MaxRequestBytes = %d, want %d", handler.MaxRequestBytes, v)
		}
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

// --- HandleGetTask ---------------------------------------------------------

const (
	testConsignmentID = "consignment-1"
	testTaskID        = "task-1"
)

// hsCodeTaskRenderConfig mirrors the shipped trade/2-hscode_selection render
// config, reduced to what this package can observe today: one state-gated
// workspace with a submit affordance.
const hsCodeTaskRenderConfig = `{
  "id": "trade-hscode-selection-flow:render",
  "sections": {
    "workspace": {
      "templateId": "form",
      "projector": "MARKDOWN",
      "visibleWhen": { "states": ["PENDING_USER"] },
      "handles": [{ "command": "submit", "label": "Complete Selection", "element": "primary_action" }]
    }
  },
  "states": { "PENDING_USER": { "actions": [{ "command": "submit" }] } }
}`

type fakeTaskFetcher struct {
	record store.TaskRecord
	found  bool
	calls  int
}

func (f *fakeTaskFetcher) GetTask(context.Context, string) (store.TaskRecord, bool) {
	f.calls++
	return f.record, f.found
}

type stubTemplates struct{ calls int }

func (s *stubTemplates) GetTemplate(context.Context, string) ([]byte, error) {
	s.calls++
	return []byte(`{"template":"body"}`), nil
}

// getTaskHandler builds a handler over a real uiprojector assembler, so an
// authorized read is asserted through the same rendering path production uses.
func getTaskHandler(t *testing.T, fetcher *fakeTaskFetcher, templates *stubTemplates) *HTTPHandler {
	t.Helper()
	asm, err := uiprojector.NewAssembler(templates, uiprojector.DefaultProjectors())
	if err != nil {
		t.Fatalf("build ui assembler: %v", err)
	}
	evaluator, err := readauthz.NewEvaluator(taskauthz.Catalog{
		Roles: map[string]string{"trader": "Trader", "cha": "CHA"},
	})
	if err != nil {
		t.Fatalf("build read evaluator: %v", err)
	}
	return &HTTPHandler{
		Store:     fetcher,
		Assembler: zoneview.NewZoneViewAssembler(zoneview.NewTaskRenderer(asm)),
		ReadAuthz: evaluator,
	}
}

func pendingHSCodeTask() *fakeTaskFetcher {
	return &fakeTaskFetcher{
		found: true,
		record: store.TaskRecord{
			TaskID:         testTaskID,
			TaskType:       "APPLICATION",
			State:          "PENDING_USER",
			RootWorkflowID: testConsignmentID,
			RenderConfig:   json.RawMessage(hsCodeTaskRenderConfig),
		},
	}
}

// ownerInput builds the Input the authz gate attaches for a user holding
// tokenRole whose company owns the consignment in ownedRole.
func ownerInput(tokenRole, ownedRole string) taskauthz.Input {
	return taskauthz.Input{
		Kind:  taskauthz.KindUser,
		Roles: []string{tokenRole},
		OwnedRoles: func(context.Context, string) (map[string]bool, error) {
			return map[string]bool{ownedRole: true}, nil
		},
	}
}

func getTask(t *testing.T, h *HTTPHandler, in *taskauthz.Input) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/tasks/"+testTaskID, nil)
	req.SetPathValue("id", testTaskID)
	if in != nil {
		req = req.WithContext(taskauthz.WithInput(req.Context(), *in))
	}
	recorder := httptest.NewRecorder()
	h.HandleGetTask(recorder, req)
	return recorder
}

func decodeZoneView(t *testing.T, body string) zoneview.ZoneView {
	t.Helper()
	var zv zoneview.ZoneView
	if err := json.Unmarshal([]byte(body), &zv); err != nil {
		t.Fatalf("decode zone view: %v (body=%s)", err, body)
	}
	return zv
}

func decodeSlots(t *testing.T, zv zoneview.ZoneView) map[string]zoneview.EnrichedComponent {
	t.Helper()
	var view map[string]zoneview.EnrichedComponent
	if err := json.Unmarshal(zv.View, &view); err != nil {
		t.Fatalf("decode view: %v", err)
	}
	return view
}

// Without the gate's Input there is no principal to authorize, so nothing about
// the task may be revealed — not even whether it exists.
func TestHandleGetTask_UnauthenticatedBeforeAnyLookup(t *testing.T) {
	fetcher := pendingHSCodeTask()
	recorder := getTask(t, getTaskHandler(t, fetcher, &stubTemplates{}), nil)

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("got %d, want 401", recorder.Code)
	}
	if fetcher.calls != 0 {
		t.Errorf("store was queried %d times, want 0", fetcher.calls)
	}
}

func TestHandleGetTask_MissingTask(t *testing.T) {
	in := ownerInput("Trader", "trader")
	recorder := getTask(t, getTaskHandler(t, &fakeTaskFetcher{}, &stubTemplates{}), &in)

	if recorder.Code != http.StatusNotFound {
		t.Fatalf("got %d, want 404", recorder.Code)
	}
}

// A caller who owns neither side gets exactly what a missing task returns, and
// the view is never assembled — no template fetch, no rendered payload.
func TestHandleGetTask_DeniedLooksIdenticalToMissing(t *testing.T) {
	templates := &stubTemplates{}
	handler := getTaskHandler(t, pendingHSCodeTask(), templates)
	auditor := &mockAuditor{}
	handler.Audit = nswaudit.NewRecorder(auditor)

	in := taskauthz.Input{
		Kind:  taskauthz.KindUser,
		Roles: []string{"Trader"},
		OwnedRoles: func(context.Context, string) (map[string]bool, error) {
			return map[string]bool{"trader": false, "cha": false}, nil
		},
	}
	recorder := getTask(t, handler, &in)

	if recorder.Code != http.StatusNotFound {
		t.Fatalf("got %d, want 404", recorder.Code)
	}
	if !strings.Contains(recorder.Body.String(), errTaskNotFound) {
		t.Errorf("body should be the not-found text, got %s", recorder.Body.String())
	}
	if templates.calls != 0 {
		t.Errorf("assembled %d templates on the denial path, want 0", templates.calls)
	}

	events := auditor.snapshot()
	if len(events) != 1 {
		t.Fatalf("recorded %d audit events, want 1", len(events))
	}
	ev := events[0]
	if ev.Status != argus.StatusFailure {
		t.Errorf("audit status = %v, want failure", ev.Status)
	}
	if ev.TargetID == nil || *ev.TargetID != testTaskID {
		t.Errorf("audit targetID = %v, want %q", ev.TargetID, testTaskID)
	}
	if ev.EventType != string(nswaudit.EventTask) || ev.Action != string(nswaudit.ActionRead) {
		t.Errorf("audit event = %s/%s, want %s/%s", ev.EventType, ev.Action, nswaudit.EventTask, nswaudit.ActionRead)
	}
}

// An M2M client owns no consignment, so it cannot read a task even holding the
// read scope.
func TestHandleGetTask_ClientPrincipalDenied(t *testing.T) {
	in := taskauthz.Input{Kind: taskauthz.KindClient, ClientID: "FCAU_TO_NSW"}
	recorder := getTask(t, getTaskHandler(t, pendingHSCodeTask(), &stubTemplates{}), &in)

	if recorder.Code != http.StatusNotFound {
		t.Fatalf("got %d, want 404", recorder.Code)
	}
}

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

func (m *mockAuditor) snapshot() []*argus.AuditLogRequest {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]*argus.AuditLogRequest(nil), m.events...)
}

func (m *mockAuditor) IsEnabled() bool { return true }

func (m *mockAuditor) SignEvent(context.Context, *argus.AuditLogRequest) error { return nil }

func (m *mockAuditor) SignMessageBytes(context.Context, []byte) (string, error) { return "", nil }

func (m *mockAuditor) LogSignedEvent(context.Context, *argus.AuditLogRequest) {}

func (m *mockAuditor) VerifyIntegrity(*argus.AuditLogRequest, crypto.PublicKey) (bool, error) {
	return true, nil
}

func (m *mockAuditor) Close(context.Context) error { return nil }

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

// The authorized path: an owner reads the task and gets the rendered view. Paired
// with the denial cases above, this is what distinguishes "the gate denies
// everyone" from "the gate denies the right people".
func TestHandleGetTask_OwnerReadsTheTask(t *testing.T) {
	fetcher := pendingHSCodeTask()
	templates := &stubTemplates{}
	in := ownerInput("Trader", "trader")

	recorder := getTask(t, getTaskHandler(t, fetcher, templates), &in)

	if recorder.Code != http.StatusOK {
		t.Fatalf("got %d, want 200: %s", recorder.Code, recorder.Body.String())
	}
	zv := decodeZoneView(t, recorder.Body.String())
	if zv.TaskID != testTaskID || zv.State != "PENDING_USER" {
		t.Errorf("got task %q in state %q, want %q/PENDING_USER", zv.TaskID, zv.State, testTaskID)
	}
	view := decodeSlots(t, zv)
	ws, ok := view["workspace"]
	if !ok {
		t.Fatalf("workspace missing from view %v", view)
	}
	if len(ws.Handles) != 1 || ws.Handles[0].Command != "submit" {
		t.Errorf("got handles %v, want the submit affordance", ws.Handles)
	}
	if templates.calls == 0 {
		t.Error("the assembler never ran, so this asserts nothing about rendering")
	}
}
