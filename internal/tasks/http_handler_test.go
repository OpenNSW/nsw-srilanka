package tasks

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/OpenNSW/core/taskflow/renderer/zoneview"
	"github.com/OpenNSW/core/taskflow/store"
	"github.com/OpenNSW/core/uiprojector"
	"github.com/OpenNSW/nsw-srilanka/internal/tasks/taskauthz"
)

func TestNewHTTPHandler_SetsMaxRequestBytes(t *testing.T) {
	for _, v := range []int64{1024, 0, -1, -33554432} {
		handler := NewHTTPHandler(nil, nil, nil, taskauthz.Catalog{}, v)
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
	return &HTTPHandler{
		Store:        fetcher,
		Assembler:    zoneview.NewZoneViewAssembler(zoneview.NewTaskRenderer(asm)),
		AuthzCatalog: taskauthz.Catalog{Roles: map[string]string{"trader": "Trader", "cha": "CHA"}},
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

// stubStore serves one record to the handler's pre-flight check.
type stubStore struct{ record store.TaskRecord }

func (s stubStore) GetTask(context.Context, string) (store.TaskRecord, bool) {
	return s.record, true
}

// ephytoRenderConfig mirrors the shape a render.json carries: the state the
// trader acts in offers a command, the states the workflow drives itself
// through offer none.
const ephytoRenderConfig = `{
  "states": {
    "PENDING_USER": { "actions": [{ "command": "submit" }] },
    "EPHYTO_SUBMITTED": { "actions": [] }
  }
}`

// A command arriving after its step has moved on is refused before it reaches
// the orchestrator, which would otherwise write the payload into whichever
// subtask is now active — overwriting the results that subtask recorded.
func TestHandleCompleteTaskStep_RefusesACommandTheStateDoesNotOffer(t *testing.T) {
	handler := &HTTPHandler{
		MaxRequestBytes: 1024,
		Store: stubStore{record: store.TaskRecord{
			TaskID:       "123",
			State:        "EPHYTO_SUBMITTED",
			RenderConfig: []byte(ephytoRenderConfig),
		}},
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/tasks/123/commands/submit", strings.NewReader(`{"hub_destination":"LK2"}`))
	req.SetPathValue("id", "123")
	req.SetPathValue("command", "submit")
	recorder := httptest.NewRecorder()

	handler.HandleCompleteTaskStep(recorder, req)

	if recorder.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d", recorder.Code)
	}
	if !strings.Contains(recorder.Body.String(), errCommandNotOffered) {
		t.Fatalf("expected error body to mention %q, got %s", errCommandNotOffered, recorder.Body.String())
	}
}

// A render config that never describes the current state leaves the decision
// where it was, so flows that predate the states contract are unaffected.
func TestOffersCommand_AllowsWhatTheConfigDoesNotDescribe(t *testing.T) {
	cases := []struct {
		name   string
		record store.TaskRecord
	}{
		{"no render config", store.TaskRecord{State: "PENDING_USER"}},
		{"unparsable render config", store.TaskRecord{State: "PENDING_USER", RenderConfig: []byte(`{`)}},
		{"no states block", store.TaskRecord{State: "PENDING_USER", RenderConfig: []byte(`{"sections":{}}`)}},
		{"state not described", store.TaskRecord{State: "SOMETHING_ELSE", RenderConfig: []byte(ephytoRenderConfig)}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if !offersCommand(c.record, "submit") {
				t.Error("want the command allowed")
			}
		})
	}
}

// The state the trader acts in still accepts its own command.
func TestOffersCommand_AllowsTheStatesOwnAction(t *testing.T) {
	record := store.TaskRecord{State: "PENDING_USER", RenderConfig: []byte(ephytoRenderConfig)}
	if !offersCommand(record, "submit") {
		t.Error("PENDING_USER offers submit")
	}
	if offersCommand(record, "acknowledge") {
		t.Error("PENDING_USER does not offer acknowledge")
	}
}
