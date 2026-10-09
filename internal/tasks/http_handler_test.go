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
	"github.com/OpenNSW/core/taskflow/callbacktoken"
	"github.com/OpenNSW/core/taskflow/renderer/zoneview"
	"github.com/OpenNSW/core/taskflow/store"
	"github.com/OpenNSW/core/uiprojector"
	nswaudit "github.com/OpenNSW/nsw-srilanka/internal/audit"
	"github.com/OpenNSW/nsw-srilanka/internal/authn"
	"github.com/OpenNSW/nsw-srilanka/internal/tasks/taskauthz"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewHTTPHandler_SetsMaxRequestBytes(t *testing.T) {
	for _, v := range []int64{1024, 0, -1, -33554432} {
		handler := NewHTTPHandler(nil, nil, nil, taskauthz.Catalog{}, nil, nil, v)
		if handler.MaxRequestBytes != v {
			t.Errorf("MaxRequestBytes = %d, want %d", handler.MaxRequestBytes, v)
		}
	}
}

func TestHandleCompleteTaskStep_RejectsOversizedBody(t *testing.T) {
	handler := &HTTPHandler{MaxRequestBytes: 8}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/tasks/123/steps/step-1", strings.NewReader(`{"command":"approve","payload":{"key":"value"}}`))
	req.SetPathValue("id", "123")
	req.SetPathValue("stepId", "step-1")
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
	req := httptest.NewRequest(http.MethodPost, "/api/v1/tasks/123/steps/step-1", strings.NewReader(`{"command":"approve","payload":{"key":"value"}}{"command":"escalate"}`))
	req.SetPathValue("id", "123")
	req.SetPathValue("stepId", "step-1")
	recorder := httptest.NewRecorder()

	handler.HandleCompleteTaskStep(recorder, req)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", recorder.Code)
	}
	if !strings.Contains(recorder.Body.String(), errInvalidRequestBody) {
		t.Fatalf("expected error body to mention %q, got %s", errInvalidRequestBody, recorder.Body.String())
	}
}

// A submission must name the step it was made against; without one it is rejected before it
// reaches the task manager (nil here, so reaching it would panic).
func TestHandleCompleteTaskStep_RequiresStepID(t *testing.T) {
	handler := &HTTPHandler{MaxRequestBytes: 1024}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/tasks/123/steps/", strings.NewReader(`{"command":"approve","payload":{"key":"value"}}`))
	req.SetPathValue("id", "123")
	recorder := httptest.NewRecorder()

	handler.HandleCompleteTaskStep(recorder, req)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", recorder.Code)
	}
}

// A callback whose token is not one core produced is rejected before it reaches the task
// manager (nil here, so reaching it would panic).
func TestHandleCompleteTaskStepByToken_RejectsMalformedToken(t *testing.T) {
	handler := &HTTPHandler{MaxRequestBytes: 1024}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/callbacks/not-a-token", strings.NewReader(`{"command":"approve","payload":{}}`))
	req.SetPathValue("token", "not-a-token")
	recorder := httptest.NewRecorder()

	handler.HandleCompleteTaskStepByToken(recorder, req)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", recorder.Code)
	}
	if !strings.Contains(recorder.Body.String(), errInvalidCallbackToken) {
		t.Fatalf("expected error body to mention %q, got %s", errInvalidCallbackToken, recorder.Body.String())
	}
}

// The token route takes the same envelope as the portal's, so a body without a command is
// rejected the same way.
func TestHandleCompleteTaskStepByToken_RequiresCommand(t *testing.T) {
	token, err := callbacktoken.Encode("6aad0417-9a6d-4407-9509-2e51d8fcae99", "ca7ed707-1dba-43ca-94bf-10eddf00df3c")
	require.NoError(t, err)
	handler := &HTTPHandler{MaxRequestBytes: 1024}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/callbacks/"+token, strings.NewReader(`{"payload":{}}`))
	req.SetPathValue("token", token)
	recorder := httptest.NewRecorder()

	handler.HandleCompleteTaskStepByToken(recorder, req)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", recorder.Code)
	}
}

// --- HandleGetTask ---------------------------------------------------------

const (
	testConsignmentID = "consignment-1"
	testTaskID        = "task-1"
)

// hsCodeTaskRenderConfig mirrors the shipped trade/2-hscode_selection render config:
// the same PENDING_USER state offers the CHA an interactive workspace and the
// trader a read-only notice, each gated on that role's ownership claim.
const hsCodeTaskRenderConfig = `{
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
	return &HTTPHandler{
		Store:        fetcher,
		Assembler:    zoneview.NewZoneViewAssembler(zoneview.NewTaskRenderer(asm)),
		AuthzCatalog: taskauthz.Catalog{Roles: map[string]string{"trader": "Trader", "cha": "CHA"}},
		Files:        NewFileBinding(newFakeFileRefs(), nil),
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

// decodeSlots indexes the rendered view by each entry's id (its section key),
// so assertions can ask which sections are present without depending on order.
func decodeSlots(t *testing.T, zv zoneview.ZoneView) map[string]zoneview.EnrichedComponent {
	t.Helper()
	var entries []zoneview.EnrichedComponent
	if err := json.Unmarshal(zv.View, &entries); err != nil {
		t.Fatalf("decode view: %v", err)
	}
	view := make(map[string]zoneview.EnrichedComponent, len(entries))
	for _, e := range entries {
		if _, dup := view[e.ID]; dup {
			t.Fatalf("duplicate view entry id %q", e.ID)
		}
		view[e.ID] = e
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

// A denied task read must emit a failure audit event matching the consignment read denial pattern.
func TestHandleGetTask_DeniedAudited(t *testing.T) {
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

	req := httptest.NewRequest(http.MethodGet, "/api/v1/tasks/"+testTaskID, nil)
	req.SetPathValue("id", testTaskID)
	ctx := taskauthz.WithInput(req.Context(), in)
	ctx = authn.ContextWithPrincipal(ctx, &authn.Principal{
		Kind:   authn.KindUser,
		UserID: "user-trader-1",
		Email:  "trader@example.com",
	})
	req = req.WithContext(ctx)

	recorder := httptest.NewRecorder()
	handler.HandleGetTask(recorder, req)

	if recorder.Code != http.StatusNotFound {
		t.Fatalf("got %d, want 404", recorder.Code)
	}

	require.Len(t, auditor.events, 1)
	assert.Equal(t, string(nswaudit.ActionRead), auditor.events[0].Action)
	assert.Equal(t, string(nswaudit.TargetTask), auditor.events[0].TargetType)
	assert.Equal(t, string(nswaudit.EventTask), auditor.events[0].EventType)
	assert.Equal(t, argus.StatusFailure, auditor.events[0].Status)
	assert.Equal(t, string(nswaudit.ActorMember), auditor.events[0].ActorType)
	assert.Equal(t, "user-trader-1", auditor.events[0].ActorID)
	assert.NotEmpty(t, auditor.events[0].Timestamp)
	require.NotNil(t, auditor.events[0].TargetID)
	assert.Equal(t, testTaskID, *auditor.events[0].TargetID)
	assert.Equal(t, "task read access denied", auditor.events[0].Metadata["error"])
}

// When handler.Audit is nil, a denied task read must not panic and still return 404.
func TestHandleGetTask_DeniedNilAuditDoesNotPanic(t *testing.T) {
	templates := &stubTemplates{}
	handler := getTaskHandler(t, pendingHSCodeTask(), templates)
	handler.Audit = nil

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
				strings.Replace(hsCodeTaskRenderConfig, `["cha", "trader"]`, tt.readRoles, 1),
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

func (m *mockAuditor) LogEvent(ctx context.Context, event *argus.AuditLogRequest) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.events = append(m.events, event)
	return true
}

func (m *mockAuditor) IsEnabled() bool { return true }

func (m *mockAuditor) SignEvent(ctx context.Context, event *argus.AuditLogRequest) error {
	return nil
}

func (m *mockAuditor) SignMessageBytes(ctx context.Context, message []byte) (string, error) {
	return "", nil
}

func (m *mockAuditor) LogSignedEvent(ctx context.Context, event *argus.AuditLogRequest) {}

func (m *mockAuditor) VerifyIntegrity(event *argus.AuditLogRequest, publicKey crypto.PublicKey) (bool, error) {
	return true, nil
}

func (m *mockAuditor) Close(ctx context.Context) error {
	return nil
}

// --- File fields -----------------------------------------------------------

const fileTaskRenderConfig = `{
  "id": "x:render",
  "files": ["form.invoice"],
  "sections": {
    "workspace": { "templateId": "form", "projector": "FORM", "dataKey": "form" }
  },
  "states": { "PENDING_USER": { "actions": [{ "command": "submit" }] } }
}`

func pendingFileTask() *fakeTaskFetcher {
	return &fakeTaskFetcher{
		found: true,
		record: store.TaskRecord{
			TaskID:               testTaskID,
			TaskType:             "APPLICATION",
			State:                "PENDING_USER",
			RootWorkflowID:       testConsignmentID,
			RenderConfig:         json.RawMessage(fileTaskRenderConfig),
			ActiveTaskTemplateID: "form-step",
			ActiveStepID:         "step-1",
			Data:                 map[string]any{"form": map[string]any{"invoice": "stored-invoice-key"}},
		},
	}
}

// A reader gets a reference to each declared file, never its stored value.
func TestHandleGetTask_GivesFileReferences(t *testing.T) {
	refs := newFakeFileRefs()
	fetcher := pendingFileTask()
	h := getTaskHandler(t, fetcher, &stubTemplates{})
	h.Files = NewFileBinding(refs, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/tasks/"+testTaskID, nil)
	req.SetPathValue("id", testTaskID)
	in := ownerInput("Trader", "trader")
	ctx := taskauthz.WithInput(req.Context(), in)
	req = req.WithContext(authn.ContextWithPrincipal(ctx, userPrincipal("alice")))
	recorder := httptest.NewRecorder()
	h.HandleGetTask(recorder, req)

	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	assert.Contains(t, recorder.Body.String(), `"invoice":"ref1"`)
	assert.NotContains(t, recorder.Body.String(), "stored-invoice-key")
	value, err := refs.Resolve(userPrincipal("alice"), "ref1")
	require.NoError(t, err)
	assert.Equal(t, "stored-invoice-key", value)
	assert.Equal(t, "stored-invoice-key", fetcher.record.Data["form"].(map[string]any)["invoice"], "the stored record must not change")
}

func TestHandleGetTask_BadFileDeclarationIsAnError(t *testing.T) {
	fetcher := pendingFileTask()
	fetcher.record.RenderConfig = json.RawMessage(`{"id":"x:render","files":["form.invoice[0]"],"sections":{}}`)
	in := ownerInput("Trader", "trader")
	recorder := getTask(t, getTaskHandler(t, fetcher, &stubTemplates{}), &in)

	assert.Equal(t, http.StatusInternalServerError, recorder.Code)
}

// submitStep posts payload to the portal route as p. The handler has no task
// manager, so a request that got past the file references would panic.
func submitStep(t *testing.T, fetcher *fakeTaskFetcher, refs *fakeFileRefs, stepID string, p *authn.Principal, payload map[string]any) *httptest.ResponseRecorder {
	t.Helper()
	var calls int
	h := &HTTPHandler{
		Store:           fetcher,
		Files:           NewFileBinding(refs, namespaces(map[string]string{"form-step": "form"}, &calls)),
		MaxRequestBytes: 1024,
	}
	body, err := json.Marshal(map[string]any{"command": "submit", "payload": payload})
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/tasks/"+testTaskID+"/steps/"+stepID, strings.NewReader(string(body)))
	req.SetPathValue("id", testTaskID)
	req.SetPathValue("stepId", stepID)
	req = req.WithContext(authn.ContextWithPrincipal(req.Context(), p))
	recorder := httptest.NewRecorder()
	h.HandleCompleteTaskStep(recorder, req)
	return recorder
}

func TestHandleCompleteTaskStep_RefusesFileReferences(t *testing.T) {
	refs := newFakeFileRefs()
	alicesRef, _ := refs.IssueFor(userPrincipal("alice"), "k")
	expiredRef, _ := refs.IssueFor(userPrincipal("bob"), "k")
	refs.expired[expiredRef] = true

	tests := []struct {
		name       string
		fetcher    *fakeTaskFetcher
		stepID     string
		payload    map[string]any
		wantStatus int
	}{
		{"missing task", &fakeTaskFetcher{}, "step-1", map[string]any{}, http.StatusNotFound},
		{"step no longer active", pendingFileTask(), "step-0", map[string]any{"invoice": alicesRef}, http.StatusConflict},
		{"someone else's file", pendingFileTask(), "step-1", map[string]any{"invoice": alicesRef}, http.StatusForbidden},
		{"a raw storage key", pendingFileTask(), "step-1", map[string]any{"invoice": "stored-invoice-key"}, http.StatusBadRequest},
		{"expired file", pendingFileTask(), "step-1", map[string]any{"invoice": expiredRef}, http.StatusGone},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recorder := submitStep(t, tt.fetcher, refs, tt.stepID, userPrincipal("bob"), tt.payload)
			assert.Equal(t, tt.wantStatus, recorder.Code, recorder.Body.String())
		})
	}
}

func TestResolveFiles_StoresTheValues(t *testing.T) {
	refs := newFakeFileRefs()
	ref, _ := refs.IssueFor(userPrincipal("alice"), "stored-invoice-key")
	var calls int
	h := &HTTPHandler{Store: pendingFileTask(), Files: NewFileBinding(refs, namespaces(map[string]string{"form-step": "form"}, &calls))}
	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req = req.WithContext(authn.ContextWithPrincipal(req.Context(), userPrincipal("alice")))
	payload := map[string]any{"invoice": ref, "__command": "submit"}

	ok := h.resolveFiles(httptest.NewRecorder(), req, testTaskID, "step-1", payload)

	require.True(t, ok)
	assert.Equal(t, map[string]any{"invoice": "stored-invoice-key", "__command": "submit"}, payload)
}
