package tasks

import (
	"context"
	"crypto"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	argus "github.com/LSFLK/argus/pkg/audit"
	"github.com/OpenNSW/core/artifact"
	"github.com/OpenNSW/core/authn"
	flowextensions "github.com/OpenNSW/core/taskflow/extensions"
	"github.com/OpenNSW/core/taskflow/orchestrator"
	flowplugins "github.com/OpenNSW/core/taskflow/plugins"
	"github.com/OpenNSW/core/taskflow/renderer/zoneview"
	"github.com/OpenNSW/core/taskflow/store"
	"github.com/OpenNSW/core/uiprojector"
	workflow "github.com/OpenNSW/core/workflow"
	nswaudit "github.com/OpenNSW/nsw-srilanka/internal/audit"
	authzext "github.com/OpenNSW/nsw-srilanka/internal/tasks/extensions/authz"
	"github.com/OpenNSW/nsw-srilanka/internal/tasks/taskauthz"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewHTTPHandler_SetsMaxRequestBytes(t *testing.T) {
	for _, v := range []int64{1024, 0, -1, -33554432} {
		handler := NewHTTPHandler(nil, nil, nil, taskauthz.Catalog{}, nil, v)
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

// completeTaskHandler wires a real TaskManager over the in-memory store with
// the authz extension registered, so the tests exercise the same PRE_RESUME
// authorization path production uses. Each case registers its own minimal
// subtask template (per-task properties) and seeds the task record.
type testTaskStore struct {
	mu     sync.Mutex
	byID   map[string]store.TaskRecord
	byWFID map[string]store.TaskRecord
}

func newTestTaskStore() *testTaskStore {
	return &testTaskStore{
		byID:   make(map[string]store.TaskRecord),
		byWFID: make(map[string]store.TaskRecord),
	}
}

func (s *testTaskStore) SaveTask(_ context.Context, record store.TaskRecord) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.byID[record.TaskID] = record
	s.byWFID[record.TaskWorkflowID] = record
}

func (s *testTaskStore) GetTask(_ context.Context, taskID string) (store.TaskRecord, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	rec, ok := s.byID[taskID]
	return rec, ok
}

func (s *testTaskStore) GetTaskByWorkflowID(_ context.Context, workflowID string) (store.TaskRecord, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	rec, ok := s.byWFID[workflowID]
	return rec, ok
}

func (s *testTaskStore) GetAllTasks(context.Context, string) []store.TaskRecord {
	return nil
}

// noopWorkflowRunner satisfies workflow.TemporalManager by doing nothing: the
// audited outcomes below are decided before any workflow interaction.
type noopWorkflowRunner struct{}

func (noopWorkflowRunner) StartWorkflow(context.Context, string, workflow.WorkflowDefinition, map[string]any) error {
	return nil
}

func (noopWorkflowRunner) TaskDone(context.Context, string, string, string, map[string]any) error {
	return nil
}

func (noopWorkflowRunner) TaskUpdate(context.Context, string, string, workflow.UpdateEvent) error {
	return nil
}

func (noopWorkflowRunner) GetStatus(context.Context, string) (*workflow.WorkflowInstance, error) {
	return nil, nil
}

func (noopWorkflowRunner) RegisterDefinitionHandler(func(string) (workflow.WorkflowDefinition, error)) {
}

func (noopWorkflowRunner) StartWorker() error { return nil }

func (noopWorkflowRunner) StopWorker() {}

// testTaskArtifactID is the id of the subtask template the test task records
// point at; the authz rules live in the template's extension properties.
const testTaskArtifactID = "test-subtask"

// testSubtaskProps names "trader" as the only logical principal allowed to run
// the "submit" command in state PENDING_USER.
const testSubtaskProps = `{"PENDING_USER": {"submit": ["trader"]}}`

// testLoader is a minimal artifact.Loader serving canned bytes by path.
type testLoader struct {
	content map[string][]byte
}

func (l *testLoader) Load(_ context.Context, path string) ([]byte, error) {
	if data, ok := l.content[path]; ok {
		return data, nil
	}
	return nil, fmt.Errorf("test artifact not found at path: %s", path)
}

// noopPlugin is a do-nothing subtask plugin; the audited outcomes below are
// decided by the authz extension before any plugin runs.
type noopPlugin struct{}

func (noopPlugin) Execute(ctx flowplugins.PluginContext, _ json.RawMessage) error {
	if ctx.Record != nil {
		ctx.Record.State = "DONE"
	}
	return nil
}

// completeTaskHandler builds the handler under test: a real TaskManager whose
// authz extension is backed by catalog, an in-memory store seeded with a task
// parked at the active subtask step, and the given audit recorder.
func completeTaskHandler(t *testing.T, catalog taskauthz.Catalog, auditor *mockAuditor) (*HTTPHandler, *testTaskStore) {
	t.Helper()

	loader := &testLoader{content: map[string][]byte{
		"subtasks/test-subtask": []byte(`{
			"id": "test-subtask",
			"task_type": "TEST_PLUGIN",
			"plugin_properties": {},
			"output_namespace": "ns",
			"extensions": [{"id": "authz", "phase": "PRE_RESUME", "properties": ` + testSubtaskProps + `}]
		}`),
	}}

	reg := artifact.NewRegistry(loader)
	reg.RegisterArtifact(testTaskArtifactID, "subtask_template", "", "subtasks/test-subtask")

	extensionsRegistry := flowextensions.NewRegistry()
	if err := authzext.Register(extensionsRegistry, catalog); err != nil {
		t.Fatalf("register authz extension: %v", err)
	}

	pluginsRegistry := flowplugins.NewRegistry()
	if err := pluginsRegistry.Register("TEST_PLUGIN", noopPlugin{}); err != nil {
		t.Fatalf("register test plugin: %v", err)
	}

	db := newTestTaskStore()
	tm := orchestrator.NewTaskManager(db, reg, pluginsRegistry, extensionsRegistry, noopWorkflowRunner{}, nil, nil)
	db.SaveTask(context.Background(), store.TaskRecord{
		TaskID:               testTaskID,
		TaskType:             "APPLICATION",
		State:                "PENDING_USER",
		RootWorkflowID:       testConsignmentID,
		TaskWorkflowID:       "task-wf-" + testTaskID,
		TaskRunID:            "run-1",
		SubTaskNodeID:        "node-1",
		ActiveTaskTemplateID: testTaskArtifactID,
	})

	return &HTTPHandler{
		Manager:         tm,
		MaxRequestBytes: 1024,
		Audit:           nswaudit.NewRecorder(auditor),
	}, db
}

// testWriteCatalog is the write-side catalog: token roles map to logical owner
// role names, mirroring the global catalog shape.
func testWriteCatalog() taskauthz.Catalog {
	return taskauthz.Catalog{
		Roles:   map[string]string{"trader": "Trader", "cha": "CHA"},
		Clients: map[string]string{"fcau": "FCAU_TO_NSW"},
	}
}

// withGateInput attaches the Input Layer 1 (authzgate) would attach, so the
// extension can resolve the caller. A nil in denotes "the gate dropped the
// principal" — the unauthenticated shape.
func withGateInput(ctx context.Context, in *taskauthz.Input) context.Context {
	if in == nil {
		return ctx
	}
	return taskauthz.WithInput(ctx, *in)
}

// completeTask performs a POST /api/v1/tasks/{id}/commands/{command} against h
// as an authenticated user, with the gate's Input attached when present.
func completeTask(t *testing.T, h *HTTPHandler, in *taskauthz.Input, command string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/tasks/"+testTaskID+"/commands/"+command, nil)
	req.SetPathValue("id", testTaskID)
	req.SetPathValue("command", command)
	ctx := context.WithValue(req.Context(), authn.AuthContextKey, &authn.AuthContext{
		User: &authn.UserContext{ID: "user-1", Roles: []string{"Trader"}},
	})
	req = req.WithContext(withGateInput(ctx, in))
	recorder := httptest.NewRecorder()
	h.HandleCompleteTaskStep(recorder, req)
	return recorder
}

// requireSingleAuditEvent asserts the shared shape of the audited outcomes and
// returns the one event for outcome-specific assertions.
func requireSingleAuditEvent(t *testing.T, auditor *mockAuditor) *argus.AuditLogRequest {
	t.Helper()
	events := auditor.getEvents()
	require.Len(t, events, 1, "expected exactly one audit event")
	ev := events[0]
	require.NotNil(t, ev.TargetID)
	assert.Equal(t, testTaskID, *ev.TargetID)
	assert.Equal(t, string(nswaudit.EventTask), ev.EventType)
	assert.Equal(t, string(nswaudit.ActionUpdate), ev.Action)
	assert.Equal(t, string(nswaudit.TargetTask), ev.TargetType)
	assert.Equal(t, string(nswaudit.ActorMember), ev.ActorType)
	assert.Equal(t, "user-1", ev.ActorID)
	assert.NotEmpty(t, ev.Timestamp)
	return ev
}

// ownedRoles resolves to the given per-role ownership flags.
func ownedRoles(owned map[string]bool) taskauthz.OwnedRolesFunc {
	return func(context.Context, string) (map[string]bool, error) {
		return owned, nil
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
	authCtx := &authn.AuthContext{
		User: &authn.UserContext{
			ID:    "user-trader-1",
			Email: "trader@example.com",
		},
	}
	ctx = context.WithValue(ctx, authn.AuthContextKey, authCtx)
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

// getEvents snapshots the recorded events.
func (m *mockAuditor) getEvents() []*argus.AuditLogRequest {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.events
}

// A 401 from the write extension (no principal on the request context) must
// emit a failure audit event, not just the slog warning.
func TestHandleCompleteTaskStep_UnauthenticatedAudited(t *testing.T) {
	auditor := &mockAuditor{}
	handler, _ := completeTaskHandler(t, testWriteCatalog(), auditor)

	// No gate Input on the context: the extension denies as unauthenticated.
	recorder := completeTask(t, handler, nil, "submit")

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("got %d, want 401", recorder.Code)
	}
	ev := requireSingleAuditEvent(t, auditor)
	assert.Equal(t, argus.StatusFailure, ev.Status)
	assert.Equal(t, "submit", ev.Metadata["command"])
	assert.Equal(t, "task_cmd_unauthenticated", ev.Metadata["error_code"])
	assert.Contains(t, ev.Metadata, "error")
}

// The extension denies a caller whose company does not own the consignment in
// an allowed role: 403 plus a failure audit event.
func TestHandleCompleteTaskStep_ForbiddenAudited(t *testing.T) {
	auditor := &mockAuditor{}
	handler, _ := completeTaskHandler(t, testWriteCatalog(), auditor)
	in := taskauthz.Input{
		Kind:       taskauthz.KindUser,
		Roles:      []string{"Trader"},
		OwnedRoles: ownedRoles(map[string]bool{"trader": false, "cha": false}),
	}

	recorder := completeTask(t, handler, &in, "submit")

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("got %d, want 403", recorder.Code)
	}
	ev := requireSingleAuditEvent(t, auditor)
	assert.Equal(t, argus.StatusFailure, ev.Status)
	assert.Equal(t, "submit", ev.Metadata["command"])
	assert.Equal(t, "task_cmd_forbidden", ev.Metadata["error_code"])
	assert.Contains(t, ev.Metadata, "error")
}

// A permitted, successfully completed command is audited as a success.
func TestHandleCompleteTaskStep_SuccessAudited(t *testing.T) {
	auditor := &mockAuditor{}
	handler, _ := completeTaskHandler(t, testWriteCatalog(), auditor)
	in := taskauthz.Input{
		Kind:       taskauthz.KindUser,
		Roles:      []string{"Trader"},
		OwnedRoles: ownedRoles(map[string]bool{"trader": true, "cha": false}),
	}

	recorder := completeTask(t, handler, &in, "submit")

	if recorder.Code != http.StatusNoContent {
		t.Fatalf("got %d, want 204: %s", recorder.Code, recorder.Body.String())
	}
	ev := requireSingleAuditEvent(t, auditor)
	assert.Equal(t, argus.StatusSuccess, ev.Status)
	assert.Equal(t, "submit", ev.Metadata["command"])
	assert.NotContains(t, ev.Metadata, "error")
}

// A (state, command) pair with no rule is deny-by-default (403), and the
// denial is audited like any other forbidden command.

// With handler.Audit nil, the write path must still deny (401/403) and serve a
// successful completion (204) without panicking, mirroring the read-path
// nil-audit guarantee: audit failures or a missing auditor never take the
// endpoint down.
func TestHandleCompleteTaskStep_NilAuditDoesNotPanic(t *testing.T) {
	auditor := &mockAuditor{}
	handler, _ := completeTaskHandler(t, testWriteCatalog(), auditor)
	handler.Audit = nil

	// 403: caller does not own the task in the required role.
	denied := taskauthz.Input{
		Kind:       taskauthz.KindUser,
		Roles:      []string{"Trader"},
		OwnedRoles: ownedRoles(map[string]bool{"trader": false, "cha": false}),
	}
	recorder := completeTask(t, handler, &denied, "submit")
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("denied request: got %d, want 403", recorder.Code)
	}

	// 401: no principal resolved on the request context.
	recorder = completeTask(t, handler, nil, "submit")
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated request: got %d, want 401", recorder.Code)
	}

	// 204: a permitted command still completes successfully.
	allowed := taskauthz.Input{
		Kind:       taskauthz.KindUser,
		Roles:      []string{"Trader"},
		OwnedRoles: ownedRoles(map[string]bool{"trader": true, "cha": false}),
	}
	recorder = completeTask(t, handler, &allowed, "submit")
	if recorder.Code != http.StatusNoContent {
		t.Fatalf("permitted request: got %d, want 204", recorder.Code)
	}
}

func TestHandleCompleteTaskStep_UnrulableCommandDeniedAndAudited(t *testing.T) {
	auditor := &mockAuditor{}
	handler, _ := completeTaskHandler(t, testWriteCatalog(), auditor)
	in := taskauthz.Input{
		Kind:       taskauthz.KindUser,
		Roles:      []string{"Trader"},
		OwnedRoles: ownedRoles(map[string]bool{"trader": true, "cha": true}),
	}

	recorder := completeTask(t, handler, &in, "escalate")

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("got %d, want 403", recorder.Code)
	}
	ev := requireSingleAuditEvent(t, auditor)
	assert.Equal(t, argus.StatusFailure, ev.Status)
	assert.Equal(t, "escalate", ev.Metadata["command"])
}
