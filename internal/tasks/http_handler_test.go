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

// hsCodeRenderConfig mirrors the shipped trade/2-hscode_selection render config:
// the same PENDING_USER state offers the CHA an interactive workspace and the
// trader a read-only notice, each gated on that role's ownership claim.
const hsCodeRenderConfig = `{
  "id": "trade-hscode-selection-flow:render",
  "read": { "roles": ["cha", "trader"] },
  "sections": {
    "status_message": {
      "templateId": "waiting",
      "projector": "MARKDOWN",
      "visibleWhen": { "states": ["PENDING_USER"], "requireClaim": "role:trader" }
    },
    "workspace": {
      "templateId": "form",
      "projector": "MARKDOWN",
      "visibleWhen": { "states": ["PENDING_USER"], "requireClaim": "role:cha" },
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

// getTaskHandler builds a handler over a real uiprojector assembler, so the
// tests exercise the same claim-gated rendering production uses.
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
			RenderConfig:   json.RawMessage(hsCodeRenderConfig),
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

// The read.roles allowlist narrows access even for a genuine owner.
func TestHandleGetTask_OwnedRoleNotAdmittedByReadPolicy(t *testing.T) {
	fetcher := pendingHSCodeTask()
	fetcher.record.RenderConfig = json.RawMessage(`{"read":{"roles":["cha"]},"sections":{}}`)
	in := ownerInput("Trader", "trader")

	recorder := getTask(t, getTaskHandler(t, fetcher, &stubTemplates{}), &in)
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("got %d, want 404", recorder.Code)
	}
}

// The headline behavior: one task, one state, two roles, two different views —
// and the submit affordance travels only with the section that owns it.
func TestHandleGetTask_ClaimsShapeTheViewPerRole(t *testing.T) {
	tests := []struct {
		name        string
		tokenRole   string
		ownedRole   string
		wantSlot    string
		wantAbsent  string
		wantHandles int
	}{
		{
			name:        "cha gets the interactive workspace",
			tokenRole:   "CHA",
			ownedRole:   "cha",
			wantSlot:    "workspace",
			wantAbsent:  "status_message",
			wantHandles: 1,
		},
		{
			name:        "trader gets the waiting notice and no affordance",
			tokenRole:   "Trader",
			ownedRole:   "trader",
			wantSlot:    "status_message",
			wantAbsent:  "workspace",
			wantHandles: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := ownerInput(tt.tokenRole, tt.ownedRole)
			recorder := getTask(t, getTaskHandler(t, pendingHSCodeTask(), &stubTemplates{}), &in)

			if recorder.Code != http.StatusOK {
				t.Fatalf("got %d, want 200: %s", recorder.Code, recorder.Body.String())
			}
			zv := decodeZoneView(t, recorder.Body.String())
			if zv.TaskID != testTaskID || zv.State != "PENDING_USER" {
				t.Errorf("got task %q state %q, want %q PENDING_USER", zv.TaskID, zv.State, testTaskID)
			}
			view := decodeSlots(t, zv)
			if len(view) != 1 {
				t.Fatalf("got %d slots %v, want exactly 1", len(view), view)
			}
			got, ok := view[tt.wantSlot]
			if !ok {
				t.Fatalf("slot %q missing from view %v", tt.wantSlot, view)
			}
			if _, ok := view[tt.wantAbsent]; ok {
				t.Errorf("slot %q must not be visible to this role", tt.wantAbsent)
			}
			if len(got.Handles) != tt.wantHandles {
				t.Errorf("got %d handles, want %d", len(got.Handles), tt.wantHandles)
			}
		})
	}
}

// A company that is both trader and CHA on one consignment still sees only the
// view for the role its token carries.
func TestHandleGetTask_SameCompanyBothSlotsFollowsTheTokenRole(t *testing.T) {
	in := taskauthz.Input{
		Kind:  taskauthz.KindUser,
		Roles: []string{"CHA"},
		OwnedRoles: func(context.Context, string) (map[string]bool, error) {
			return map[string]bool{"trader": true, "cha": true}, nil
		},
	}
	recorder := getTask(t, getTaskHandler(t, pendingHSCodeTask(), &stubTemplates{}), &in)

	if recorder.Code != http.StatusOK {
		t.Fatalf("got %d, want 200: %s", recorder.Code, recorder.Body.String())
	}
	view := decodeSlots(t, decodeZoneView(t, recorder.Body.String()))
	if _, ok := view["workspace"]; !ok {
		t.Errorf("want the workspace for a CHA token, got %v", view)
	}
	if _, ok := view["status_message"]; ok {
		t.Error("the trader notice must not be visible to a CHA token")
	}
}

// A self-clearing operator — one user holding both roles at a company filling
// both slots — is genuinely eligible for both. Without precedence they were
// served the CHA form and the "waiting for your CHA" notice on the same screen.
// read.roles order must resolve that to exactly one section.
func TestHandleGetTask_DualRoleUserGetsOneViewNotBoth(t *testing.T) {
	bothRoles := func() taskauthz.Input {
		return taskauthz.Input{
			Kind:  taskauthz.KindUser,
			Roles: []string{"Trader", "CHA"},
			OwnedRoles: func(context.Context, string) (map[string]bool, error) {
				return map[string]bool{"trader": true, "cha": true}, nil
			},
		}
	}

	tests := []struct {
		name       string
		readRoles  string
		wantSlot   string
		wantAbsent string
	}{
		{
			// The shipped HS-code config: the CHA is the actor, so they get the form.
			name:       "cha first yields the workspace",
			readRoles:  `["cha", "trader"]`,
			wantSlot:   "workspace",
			wantAbsent: "status_message",
		},
		{
			name:       "trader first yields the notice",
			readRoles:  `["trader", "cha"]`,
			wantSlot:   "status_message",
			wantAbsent: "workspace",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fetcher := pendingHSCodeTask()
			fetcher.record.RenderConfig = json.RawMessage(
				strings.Replace(hsCodeRenderConfig, `["cha", "trader"]`, tt.readRoles, 1),
			)
			in := bothRoles()

			recorder := getTask(t, getTaskHandler(t, fetcher, &stubTemplates{}), &in)
			if recorder.Code != http.StatusOK {
				t.Fatalf("got %d, want 200: %s", recorder.Code, recorder.Body.String())
			}
			view := decodeSlots(t, decodeZoneView(t, recorder.Body.String()))
			if len(view) != 1 {
				t.Fatalf("got %d slots %v, want exactly 1 — a dual-role user must not see both views", len(view), view)
			}
			if _, ok := view[tt.wantSlot]; !ok {
				t.Errorf("slot %q missing from view %v", tt.wantSlot, view)
			}
			if _, ok := view[tt.wantAbsent]; ok {
				t.Errorf("slot %q must not be rendered alongside %q", tt.wantAbsent, tt.wantSlot)
			}
		})
	}
}

// A render config naming a claim the app cannot produce is a configuration bug.
// It must surface as a 500, not quietly hide the section.
func TestHandleGetTask_UnknownClaimInConfigIsAnError(t *testing.T) {
	fetcher := pendingHSCodeTask()
	fetcher.record.RenderConfig = json.RawMessage(`{
	  "read": { "roles": ["trader"] },
	  "sections": {
	    "workspace": {
	      "templateId": "form",
	      "projector": "MARKDOWN",
	      "visibleWhen": { "requireClaim": "role:chaa" }
	    }
	  }
	}`)
	in := ownerInput("Trader", "trader")

	recorder := getTask(t, getTaskHandler(t, fetcher, &stubTemplates{}), &in)
	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("got %d, want 500: %s", recorder.Code, recorder.Body.String())
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
