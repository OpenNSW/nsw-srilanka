package tasks

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/OpenNSW/core/artifact"
	"github.com/OpenNSW/core/artifact/testutil"
	"github.com/OpenNSW/core/httputil"
	"github.com/OpenNSW/core/taskflow/callbacktoken"
	"github.com/OpenNSW/core/taskflow/orchestrator"
	"github.com/OpenNSW/core/taskflow/renderconfig"
	"github.com/OpenNSW/core/taskflow/renderer/zoneview"
	"github.com/OpenNSW/core/taskflow/store"
	"github.com/OpenNSW/core/uiprojector"
	nswaudit "github.com/OpenNSW/nsw-srilanka/internal/audit"
	"github.com/OpenNSW/nsw-srilanka/internal/authn"
	"github.com/OpenNSW/nsw-srilanka/internal/storage/fileaccess"
	"github.com/OpenNSW/nsw-srilanka/internal/tasks/extensions/payloadschema"
	"github.com/OpenNSW/nsw-srilanka/internal/tasks/taskauthz"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewHTTPHandler_SetsMaxRequestBytes(t *testing.T) {
	for _, v := range []int64{1024, 0, -1, -33554432} {
		handler := NewHTTPHandler(nil, nil, nil, taskauthz.Catalog{}, nil, nil, nil, nil, v)
		if handler.MaxRequestBytes != v {
			t.Errorf("MaxRequestBytes = %d, want %d", handler.MaxRequestBytes, v)
		}
	}
}

func TestHandleCompleteTaskStep_RejectsOversizedBody(t *testing.T) {
	handler := &HTTPHandler{MaxRequestBytes: 8, Store: pendingFileTask(), CanAct: mayAct}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/tasks/"+testTaskID+"/steps/step-1", strings.NewReader(`{"command":"approve","payload":{"key":"value"}}`))
	req.SetPathValue("id", testTaskID)
	req.SetPathValue("stepId", "step-1")
	req = asCaller(req, userPrincipal("bob"))
	recorder := httptest.NewRecorder()

	handler.HandleCompleteTaskStep(recorder, req)

	if recorder.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("expected 413, got %d", recorder.Code)
	}
	if !strings.Contains(recorder.Body.String(), httputil.MsgBodyTooLarge) {
		t.Fatalf("expected error body to mention %q, got %s", httputil.MsgBodyTooLarge, recorder.Body.String())
	}
}

func TestHandleCompleteTaskStep_RejectsTrailingDataAfterJSON(t *testing.T) {
	handler := &HTTPHandler{MaxRequestBytes: 1024, Store: pendingFileTask(), CanAct: mayAct}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/tasks/"+testTaskID+"/steps/step-1", strings.NewReader(`{"command":"approve","payload":{"key":"value"}}{"command":"escalate"}`))
	req.SetPathValue("id", testTaskID)
	req.SetPathValue("stepId", "step-1")
	req = asCaller(req, userPrincipal("bob"))
	recorder := httptest.NewRecorder()

	handler.HandleCompleteTaskStep(recorder, req)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", recorder.Code)
	}
	if !strings.Contains(recorder.Body.String(), httputil.MsgInvalidBody) {
		t.Fatalf("expected error body to mention %q, got %s", httputil.MsgInvalidBody, recorder.Body.String())
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
	client, capture := nswaudit.NewWithCapture()
	handler := &HTTPHandler{MaxRequestBytes: 1024, Audit: client}
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
	recs := capture.Records()
	require.Len(t, recs, 1)
	assert.Equal(t, nswaudit.StatusFailure, recs[0].Status)
	assert.Equal(t, "invalid_callback_token", recs[0].Metadata["error"])
}

// The token route takes the same envelope as the portal's, so a body without a command is
// rejected the same way.
func TestHandleCompleteTaskStepByToken_RequiresCommand(t *testing.T) {
	token, fetcher := callbackTask(t)
	handler := &HTTPHandler{MaxRequestBytes: 1024, Store: fetcher, CanAct: mayAct}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/callbacks/"+token, strings.NewReader(`{"payload":{}}`))
	req.SetPathValue("token", token)
	req = asCaller(req, userPrincipal("bob"))
	recorder := httptest.NewRecorder()

	handler.HandleCompleteTaskStepByToken(recorder, req)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", recorder.Code)
	}
}

func TestHandleCompleteTaskStep_AuditParseFailure(t *testing.T) {
	client, capture := nswaudit.NewWithCapture()
	handler := NewHTTPHandler(nil, pendingFileTask(), nil, taskauthz.Catalog{}, nil, mayAct, nil, client, 1024)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/tasks/"+testTaskID+"/steps/step-1", strings.NewReader(`{invalid`))
	req.SetPathValue("id", testTaskID)
	req.SetPathValue("stepId", "step-1")
	req = asCaller(req, userPrincipal("bob"))
	rec := httptest.NewRecorder()

	handler.HandleCompleteTaskStep(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code)
	recs := capture.Records()
	require.Len(t, recs, 1)
	assert.Equal(t, nswaudit.EventTask, recs[0].EventType)
	assert.Equal(t, nswaudit.StatusFailure, recs[0].Status)
	assert.Equal(t, testTaskID, recs[0].TargetID)
	assert.Equal(t, http.StatusBadRequest, recs[0].Metadata["status"])
	assert.Equal(t, "step-1", recs[0].Metadata["stepId"])
}

func TestWriteCompletion_DraftAndRejectedPayloadAreAudited(t *testing.T) {
	req := httptest.NewRequest(http.MethodPut, "/", nil)

	t.Run("draft", func(t *testing.T) {
		client, capture := nswaudit.NewWithCapture()
		h := &HTTPHandler{Audit: client}
		h.writeCompletion(httptest.NewRecorder(), req, nil, "taskId", "t1", "stepId", "s1", "draft", true)
		recs := capture.Records()
		require.Len(t, recs, 1)
		assert.Equal(t, nswaudit.StatusSuccess, recs[0].Status)
		assert.Equal(t, "t1", recs[0].TargetID)
		assert.Equal(t, "s1", recs[0].Metadata["stepId"])
		assert.Equal(t, true, recs[0].Metadata["draft"])
		_, hasCommand := recs[0].Metadata["command"]
		assert.False(t, hasCommand)
	})

	t.Run("invalid payload", func(t *testing.T) {
		client, capture := nswaudit.NewWithCapture()
		h := &HTTPHandler{Audit: client}
		h.writeCompletion(httptest.NewRecorder(), req, payloadschema.ErrInvalidPayload, "taskId", "t1", "stepId", "s1", "command", "submit")
		recs := capture.Records()
		require.Len(t, recs, 1)
		assert.Equal(t, nswaudit.StatusFailure, recs[0].Status)
		assert.Equal(t, "invalid_payload", recs[0].Metadata["error"])
		assert.Equal(t, "s1", recs[0].Metadata["stepId"])
		assert.Equal(t, "submit", recs[0].Metadata["command"])
	})

	t.Run("draft not enabled", func(t *testing.T) {
		client, capture := nswaudit.NewWithCapture()
		h := &HTTPHandler{Audit: client}
		h.writeCompletion(httptest.NewRecorder(), req, orchestrator.ErrDraftNotEnabled, "taskId", "t1", "stepId", "s1", "draft", true)
		recs := capture.Records()
		require.Len(t, recs, 1)
		assert.Equal(t, "draft_not_enabled", recs[0].Metadata["error"])
		assert.Equal(t, true, recs[0].Metadata["draft"])
		assert.Equal(t, "s1", recs[0].Metadata["stepId"])
	})
}

// --- HandleGetTask ---------------------------------------------------------

const (
	testConsignmentID = "consignment-1"
	testTaskID        = "task-1"
)

// hsCodeTaskRenderConfig mirrors the shipped trade/2-hscode_selection render config:
// the same PENDING_USER state offers the CHA an interactive workspace and the
// trader a read-only notice, each gated on that role's ownership claim. Its task
// template reads hsCodeTaskReadRoles.
const hsCodeTaskRenderConfig = `{
  "id": "trade-hscode-selection-flow:render",
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

// hsCodeTaskReadRoles is the shipped trade/2-hscode_selection task template's read.roles.
const hsCodeTaskReadRoles = `["cha", "trader"]`

type fakeTaskFetcher struct {
	record store.TaskRecord
	found  bool
	// err, when set, is a failed read: GetTask returns it whatever found says.
	err   error
	calls int
	// renderConfig is the render config of record's task template, testTaskTemplateID.
	renderConfig string
	// readRoles is that task template's read.roles, as a JSON array, or "" for none.
	readRoles string
	// claimable is that task template's claimable.
	claimable bool
}

// testTaskTemplateID is the task template of the test tasks, in manifest version 1.
const testTaskTemplateID = "test-task"

// renderConfigOf resolves record's render config through manifest version 1 of an
// artifact set holding the task template testTaskTemplateID, which renders as
// f.renderConfig, is read by f.readRoles and is claimable when f.claimable. It also
// returns the set, for the zone view assembler and the task template resolver.
func (f *fakeTaskFetcher) renderConfigOf(t *testing.T) (RenderConfigOf, *artifact.Set) {
	t.Helper()
	read := ""
	if f.readRoles != "" {
		read = `,"read":{"roles":` + f.readRoles + `}`
	}
	if f.claimable {
		read += `,"claimable":true`
	}
	set, err := artifact.LoadSet(context.Background(), testutil.MemLoader{
		"manifest_v1.json": []byte(`{"artifacts":[
			{"id":"` + testTaskTemplateID + `","kind":"task_template","path":"task_template.json"},
			{"id":"` + testTaskTemplateID + `:render","kind":"generic_template","path":"render.json"}
		]}`),
		"task_template.json": []byte(`{"id":"` + testTaskTemplateID + `","type":"APPLICATION","workflow_id":"test-task-flow","render_config_id":"` + testTaskTemplateID + `:render"` + read + `}`),
		"render.json":        []byte(f.renderConfig),
	})
	require.NoError(t, err)
	return func(ctx context.Context, record store.TaskRecord) (json.RawMessage, error) {
		return renderconfig.Load(ctx, set, record)
	}, set
}

func (f *fakeTaskFetcher) GetTask(_ context.Context, taskID string) (store.TaskRecord, error) {
	f.calls++
	if f.err != nil {
		return store.TaskRecord{}, f.err
	}
	if !f.found {
		return store.TaskRecord{}, fmt.Errorf("%w: %s", store.ErrTaskNotFound, taskID)
	}
	return f.record, nil
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
	renderConfigOf, set := fetcher.renderConfigOf(t)
	return &HTTPHandler{
		Store:          fetcher,
		Assembler:      zoneview.NewZoneViewAssembler(zoneview.NewTaskRenderer(asm), set),
		AuthzCatalog:   taskauthz.Catalog{Roles: map[string]string{"trader": "Trader", "cha": "CHA"}},
		Files:          NewFileBinding(newFakeFileRefs(), nil, renderConfigOf),
		TaskTemplateOf: TaskTemplates(set),
		RenderConfigOf: renderConfigOf,
	}
}

func pendingHSCodeTask() *fakeTaskFetcher {
	return &fakeTaskFetcher{
		found: true,
		record: store.TaskRecord{
			TaskID:   testTaskID,
			TaskType: "APPLICATION",
			State:    "PENDING_USER",
			CaseID:   testConsignmentID,
			// The task template's render config is the fetcher's renderConfig.
			TaskTemplateID:  testTaskTemplateID,
			ManifestVersion: 1,
		},
		renderConfig: hsCodeTaskRenderConfig,
		readRoles:    hsCodeTaskReadRoles,
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

// A task that could not be read is a server error, so the client retries, not a 404.
func TestHandleGetTask_FailedReadIsNotMissing(t *testing.T) {
	in := ownerInput("Trader", "trader")
	recorder := getTask(t, getTaskHandler(t, &fakeTaskFetcher{err: errors.New("db down")}, &stubTemplates{}), &in)

	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("got %d, want 500", recorder.Code)
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
	client, capture := nswaudit.NewWithCapture()
	handler.Audit = client

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

	recs := capture.Records()
	require.Len(t, recs, 1)
	assert.Equal(t, nswaudit.ActionRead, recs[0].Action)
	assert.Equal(t, nswaudit.TargetTask, recs[0].TargetType)
	assert.Equal(t, nswaudit.EventTask, recs[0].EventType)
	assert.Equal(t, nswaudit.StatusFailure, recs[0].Status)
	assert.Equal(t, nswaudit.ActorMember, recs[0].ActorType)
	assert.Equal(t, "user-trader-1", recs[0].ActorID)
	assert.NotEmpty(t, recs[0].Timestamp)
	assert.Equal(t, testTaskID, recs[0].TargetID)
	assert.Equal(t, "task read access denied", recs[0].Metadata["error"])
}

// A successful task read must emit a success audit event (counterpart to DeniedAudited).
func TestHandleGetTask_SuccessAudited(t *testing.T) {
	handler := getTaskHandler(t, pendingHSCodeTask(), &stubTemplates{})
	client, capture := nswaudit.NewWithCapture()
	handler.Audit = client

	in := ownerInput("Trader", "trader")
	req := httptest.NewRequest(http.MethodGet, "/api/v1/tasks/"+testTaskID, nil)
	req.SetPathValue("id", testTaskID)
	ctx := taskauthz.WithInput(req.Context(), in)
	ctx = authn.ContextWithPrincipal(ctx, &authn.Principal{
		Kind:   authn.KindUser,
		UserID: "user-trader-1",
	})
	req = req.WithContext(ctx)

	recorder := httptest.NewRecorder()
	handler.HandleGetTask(recorder, req)

	require.Equal(t, http.StatusOK, recorder.Code)
	recs := capture.Records()
	require.Len(t, recs, 1)
	assert.Equal(t, nswaudit.EventTask, recs[0].EventType)
	assert.Equal(t, nswaudit.ActionRead, recs[0].Action)
	assert.Equal(t, nswaudit.TargetTask, recs[0].TargetType)
	assert.Equal(t, nswaudit.StatusSuccess, recs[0].Status)
	assert.Equal(t, testTaskID, recs[0].TargetID)
	assert.Equal(t, "user-trader-1", recs[0].ActorID)
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
	fetcher.readRoles = `["cha"]`
	in := ownerInput("Trader", "trader")

	recorder := getTask(t, getTaskHandler(t, fetcher, &stubTemplates{}), &in)
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("got %d, want 404", recorder.Code)
	}
}

// A task template that does not load is a real failure, not a denial: a 500 says
// "fix the template", a 404 would quietly hide the task instead.
func TestHandleGetTask_MalformedTaskTemplateIsNotADenial(t *testing.T) {
	fetcher := pendingHSCodeTask()
	fetcher.readRoles = `"trader"` // a string, where read.roles is a list
	in := ownerInput("Trader", "trader")

	recorder := getTask(t, getTaskHandler(t, fetcher, &stubTemplates{}), &in)
	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("got %d, want 500: %s", recorder.Code, recorder.Body.String())
	}
}

// The task view names the task by its render config's title, else by its task template.
func TestHandleGetTask_Title(t *testing.T) {
	for name, tc := range map[string]struct {
		renderConfig string
		want         string
	}{
		"render config title":         {`{"id":"x:render","title":"Select HS codes","sections":{}}`, "Select HS codes"},
		"no title, the task template": {`{"id":"x:render","sections":{}}`, testTaskTemplateID},
	} {
		t.Run(name, func(t *testing.T) {
			fetcher := pendingHSCodeTask()
			fetcher.renderConfig = tc.renderConfig
			in := ownerInput("Trader", "trader")

			recorder := getTask(t, getTaskHandler(t, fetcher, &stubTemplates{}), &in)
			require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
			var body struct {
				Title string `json:"title"`
			}
			require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &body))
			assert.Equal(t, tc.want, body.Title)
		})
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
			fetcher.readRoles = tt.readRoles
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
	fetcher.readRoles = `["trader"]`
	fetcher.renderConfig = `{
	  "sections": {
	    "workspace": {
	      "templateId": "form",
	      "projector": "MARKDOWN",
	      "visibleWhen": { "requireClaim": "role:chaa" }
	    }
	  }
	}`
	in := ownerInput("Trader", "trader")

	recorder := getTask(t, getTaskHandler(t, fetcher, &stubTemplates{}), &in)
	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("got %d, want 500: %s", recorder.Code, recorder.Body.String())
	}
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
			CaseID:               testConsignmentID,
			TaskTemplateID:       testTaskTemplateID,
			ManifestVersion:      1,
			ActiveTaskTemplateID: "form-step",
			ActiveStepID:         "step-1",
			Data:                 map[string]any{"form": map[string]any{"invoice": "stored-invoice-key"}},
		},
		renderConfig: fileTaskRenderConfig,
	}
}

// A reader gets a reference to each declared file, never its stored value.
// It can be submitted back only when the reader can act on the step.
func TestHandleGetTask_GivesFileReferences(t *testing.T) {
	readOnly := []fileaccess.Scope{fileaccess.ScopeRead}
	tests := []struct {
		name       string
		canAct     CanAct
		wantScopes []fileaccess.Scope
	}{
		{"reader who can act", func(context.Context, store.TaskRecord, taskauthz.Input) (bool, error) { return true, nil }, readSubmit},
		{"reader who can't act", func(context.Context, store.TaskRecord, taskauthz.Input) (bool, error) { return false, nil }, readOnly},
		{"failure to decide", func(context.Context, store.TaskRecord, taskauthz.Input) (bool, error) {
			return true, errors.New("registry down")
		}, readOnly},
		{"no CanAct", nil, readOnly},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			refs := newFakeFileRefs()
			fetcher := pendingFileTask()
			h := getTaskHandler(t, fetcher, &stubTemplates{})
			renderConfigOf, _ := fetcher.renderConfigOf(t)
			h.Files = NewFileBinding(refs, nil, renderConfigOf)
			var asked taskauthz.Input
			if tt.canAct != nil {
				h.CanAct = func(ctx context.Context, record store.TaskRecord, in taskauthz.Input) (bool, error) {
					assert.Equal(t, testTaskID, record.TaskID)
					asked = in
					return tt.canAct(ctx, record, in)
				}
			}

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
			value, err := refs.Resolve(userPrincipal("alice"), "ref1", fileaccess.ScopeRead)
			require.NoError(t, err)
			assert.Equal(t, "stored-invoice-key", value)
			assert.Equal(t, tt.wantScopes, refs.issued["ref1"].scopes)
			if tt.canAct != nil {
				assert.Equal(t, in.Roles, asked.Roles, "CanAct must be asked about the reader")
			}
			assert.Equal(t, "stored-invoice-key", fetcher.record.Data["form"].(map[string]any)["invoice"], "the stored record must not change")
		})
	}
}

func TestHandleGetTask_BadFileDeclarationIsAnError(t *testing.T) {
	fetcher := pendingFileTask()
	fetcher.renderConfig = `{"id":"x:render","files":["form.invoice[0]"],"sections":{}}`
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
		Files:           NewFileBinding(refs, namespaces(map[string]string{"form-step": "form"}, &calls), fixedRenderConfig(fetcher.renderConfig)),
		CanAct:          mayAct,
		MaxRequestBytes: 1024,
	}
	body, err := json.Marshal(map[string]any{"command": "submit", "payload": payload})
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/tasks/"+testTaskID+"/steps/"+stepID, strings.NewReader(string(body)))
	req.SetPathValue("id", testTaskID)
	req.SetPathValue("stepId", stepID)
	req = asCaller(req, p)
	recorder := httptest.NewRecorder()
	h.HandleCompleteTaskStep(recorder, req)
	return recorder
}

func TestHandleCompleteTaskStep_RefusesFileReferences(t *testing.T) {
	refs := newFakeFileRefs()
	alicesRef, _ := refs.Issue(userPrincipal("alice"), "k", readSubmit...)
	expiredRef, _ := refs.Issue(userPrincipal("bob"), "k", readSubmit...)
	refs.expired[expiredRef] = true
	readOnlyRef, _ := refs.Issue(userPrincipal("bob"), "k", fileaccess.ScopeRead)

	tests := []struct {
		name       string
		fetcher    *fakeTaskFetcher
		stepID     string
		payload    map[string]any
		wantStatus int
	}{
		{"missing task", &fakeTaskFetcher{}, "step-1", map[string]any{}, http.StatusNotFound},
		{"task could not be read", &fakeTaskFetcher{err: errors.New("db down")}, "step-1", map[string]any{}, http.StatusInternalServerError},
		{"step no longer active", pendingFileTask(), "step-0", map[string]any{"invoice": alicesRef}, http.StatusConflict},
		{"someone else's file", pendingFileTask(), "step-1", map[string]any{"invoice": alicesRef}, http.StatusForbidden},
		{"a raw storage key", pendingFileTask(), "step-1", map[string]any{"invoice": "stored-invoice-key"}, http.StatusBadRequest},
		{"expired file", pendingFileTask(), "step-1", map[string]any{"invoice": expiredRef}, http.StatusGone},
		{"a file that can't be submitted", pendingFileTask(), "step-1", map[string]any{"invoice": readOnlyRef}, http.StatusForbidden},
		{"a list where one file is declared", pendingFileTask(), "step-1", map[string]any{"invoice": []any{alicesRef, expiredRef}}, http.StatusBadRequest},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recorder := submitStep(t, tt.fetcher, refs, tt.stepID, userPrincipal("bob"), tt.payload)
			assert.Equal(t, tt.wantStatus, recorder.Code, recorder.Body.String())
		})
	}
}

func TestHandleCompleteTaskStep_AuditsFileDenial(t *testing.T) {
	refs := newFakeFileRefs()
	alicesRef, err := refs.Issue(userPrincipal("alice"), "k", readSubmit...)
	require.NoError(t, err)
	client, capture := nswaudit.NewWithCapture()
	var calls int
	h := &HTTPHandler{
		Store:           pendingFileTask(),
		Files:           NewFileBinding(refs, namespaces(map[string]string{"form-step": "form"}, &calls), fixedRenderConfig(fileTaskRenderConfig)),
		CanAct:          mayAct,
		MaxRequestBytes: 1024,
		Audit:           client,
	}
	body, err := json.Marshal(map[string]any{"command": "submit", "payload": map[string]any{"invoice": alicesRef}})
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/tasks/"+testTaskID+"/steps/step-1", strings.NewReader(string(body)))
	req.SetPathValue("id", testTaskID)
	req.SetPathValue("stepId", "step-1")
	req = asCaller(req, userPrincipal("bob"))

	h.HandleCompleteTaskStep(httptest.NewRecorder(), req)

	recs := capture.Records()
	require.Len(t, recs, 1)
	assert.Equal(t, nswaudit.StatusFailure, recs[0].Status)
	assert.Equal(t, testTaskID, recs[0].TargetID)
	assert.Equal(t, "step-1", recs[0].Metadata["stepId"])
	assert.Equal(t, "file_not_yours", recs[0].Metadata["error"])
	assert.Equal(t, http.StatusForbidden, recs[0].Metadata["status"])
	assert.Equal(t, "submit", recs[0].Metadata["command"])
}

// A submission for a step the task has left is refused before its body is read, so its
// audit record names the step but not the command.
func TestHandleCompleteTaskStep_AuditsStaleStepBeforeReadingTheBody(t *testing.T) {
	client, capture := nswaudit.NewWithCapture()
	var calls int
	h := &HTTPHandler{
		Store:           pendingFileTask(),
		Files:           NewFileBinding(newFakeFileRefs(), namespaces(map[string]string{"form-step": "form"}, &calls), fixedRenderConfig(fileTaskRenderConfig)),
		CanAct:          mayAct,
		MaxRequestBytes: 1024,
		Audit:           client,
	}
	body := &countingReader{r: strings.NewReader(`{"command":"submit","payload":{}}`)}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/tasks/"+testTaskID+"/steps/step-0", body)
	req.SetPathValue("id", testTaskID)
	req.SetPathValue("stepId", "step-0")
	req = asCaller(req, userPrincipal("bob"))
	rec := httptest.NewRecorder()

	h.HandleCompleteTaskStep(rec, req)

	require.Equal(t, http.StatusConflict, rec.Code)
	assert.Zero(t, body.n, "body bytes read")
	recs := capture.Records()
	require.Len(t, recs, 1)
	assert.Equal(t, nswaudit.StatusFailure, recs[0].Status)
	assert.Equal(t, "stale_step", recs[0].Metadata["error"])
	assert.Equal(t, "step-0", recs[0].Metadata["stepId"])
	_, hasCommand := recs[0].Metadata["command"]
	assert.False(t, hasCommand)
}

func TestHandleSaveDraft_AuditsStaleStepAsDraft(t *testing.T) {
	client, capture := nswaudit.NewWithCapture()
	var calls int
	h := &HTTPHandler{
		Store:           pendingFileTask(),
		Files:           NewFileBinding(newFakeFileRefs(), namespaces(map[string]string{"form-step": "form"}, &calls), fixedRenderConfig(fileTaskRenderConfig)),
		CanAct:          mayAct,
		MaxRequestBytes: 1024,
		Audit:           client,
	}
	req := httptest.NewRequest(http.MethodPut, "/api/v1/tasks/"+testTaskID+"/steps/step-0/draft", strings.NewReader(`{"payload":{}}`))
	req.SetPathValue("id", testTaskID)
	req.SetPathValue("stepId", "step-0")
	req = asCaller(req, userPrincipal("bob"))
	rec := httptest.NewRecorder()

	h.HandleSaveDraft(rec, req)

	require.Equal(t, http.StatusConflict, rec.Code)
	recs := capture.Records()
	require.Len(t, recs, 1)
	assert.Equal(t, nswaudit.StatusFailure, recs[0].Status)
	assert.Equal(t, "stale_step", recs[0].Metadata["error"])
	assert.Equal(t, "step-0", recs[0].Metadata["stepId"])
	assert.Equal(t, true, recs[0].Metadata["draft"])
	_, hasCommand := recs[0].Metadata["command"]
	assert.False(t, hasCommand)
}

func TestResolveFiles_StoresTheValues(t *testing.T) {
	refs := newFakeFileRefs()
	ref, _ := refs.Issue(userPrincipal("alice"), "stored-invoice-key", readSubmit...)
	var calls int
	h := &HTTPHandler{Store: pendingFileTask(), Files: NewFileBinding(refs, namespaces(map[string]string{"form-step": "form"}, &calls), fixedRenderConfig(fileTaskRenderConfig))}
	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req = req.WithContext(authn.ContextWithPrincipal(req.Context(), userPrincipal("alice")))
	payload := map[string]any{"invoice": ref, "__command": "submit"}

	ok := h.resolveFiles(httptest.NewRecorder(), req, pendingFileTask().record, payload, []any{"taskId", testTaskID, "stepId", "step-1", "command", payload["__command"]})

	require.True(t, ok)
	assert.Equal(t, map[string]any{"invoice": "stored-invoice-key", "__command": "submit"}, payload)
}

// --- HandleSaveDraft -------------------------------------------------------

// saveDraft puts body to the draft route as p. The handler has no task manager,
// so a request that got past the checks before it would panic.
func saveDraft(t *testing.T, fetcher *fakeTaskFetcher, refs *fakeFileRefs, stepID string, p *authn.Principal, body string) *httptest.ResponseRecorder {
	t.Helper()
	var calls int
	h := &HTTPHandler{
		Store:           fetcher,
		Files:           NewFileBinding(refs, namespaces(map[string]string{"form-step": "form"}, &calls), fixedRenderConfig(fetcher.renderConfig)),
		CanAct:          mayAct,
		MaxRequestBytes: 1024,
	}
	req := httptest.NewRequest(http.MethodPut, "/api/v1/tasks/"+testTaskID+"/steps/"+stepID+"/draft", strings.NewReader(body))
	req.SetPathValue("id", testTaskID)
	req.SetPathValue("stepId", stepID)
	req = asCaller(req, p)
	recorder := httptest.NewRecorder()
	h.HandleSaveDraft(recorder, req)
	return recorder
}

func TestHandleSaveDraft_RejectsBadRequests(t *testing.T) {
	refs := newFakeFileRefs()
	alicesRef, _ := refs.Issue(userPrincipal("alice"), "k", readSubmit...)

	tests := []struct {
		name       string
		fetcher    *fakeTaskFetcher
		stepID     string
		body       string
		wantStatus int
	}{
		{"no step id", pendingFileTask(), "", `{"payload":{}}`, http.StatusBadRequest},
		{"a command: the step's template decides it", pendingFileTask(), "step-1", `{"command":"submit","payload":{}}`, http.StatusBadRequest},
		{"the reserved command key", pendingFileTask(), "step-1", `{"payload":{"__command":"submit"}}`, http.StatusBadRequest},
		{"trailing data", pendingFileTask(), "step-1", `{"payload":{}}{"payload":{}}`, http.StatusBadRequest},
		{"oversized", pendingFileTask(), "step-1", `{"payload":{"x":"` + strings.Repeat("a", 2048) + `"}}`, http.StatusRequestEntityTooLarge},
		{"missing task", &fakeTaskFetcher{}, "step-1", `{"payload":{}}`, http.StatusNotFound},
		{"step no longer active", pendingFileTask(), "step-0", `{"payload":{}}`, http.StatusConflict},
		{"someone else's file", pendingFileTask(), "step-1", `{"payload":{"invoice":"` + alicesRef + `"}}`, http.StatusForbidden},
		{"a raw storage key", pendingFileTask(), "step-1", `{"payload":{"invoice":"stored-invoice-key"}}`, http.StatusBadRequest},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recorder := saveDraft(t, tt.fetcher, refs, tt.stepID, userPrincipal("bob"), tt.body)
			assert.Equal(t, tt.wantStatus, recorder.Code, recorder.Body.String())
		})
	}
}

func TestWriteCompletion_DraftNotEnabledIsABadRequest(t *testing.T) {
	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/", nil)

	(&HTTPHandler{}).writeCompletion(recorder, req, fmt.Errorf("%w: step template %q", orchestrator.ErrDraftNotEnabled, "x"))

	assert.Equal(t, http.StatusBadRequest, recorder.Code)
	assert.Contains(t, recorder.Body.String(), errDraftNotEnabled)
}

// A step whose template names no one who may act is closed to everyone.
func TestWriteCompletion_StepNotGuardedIsForbidden(t *testing.T) {
	client, capture := nswaudit.NewWithCapture()
	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/", nil)

	(&HTTPHandler{Audit: client}).writeCompletion(recorder, req, fmt.Errorf("%w: step template %q", orchestrator.ErrStepNotGuarded, "x"), "taskId", "t1", "stepId", "s1")

	assert.Equal(t, http.StatusForbidden, recorder.Code)
	assert.Contains(t, recorder.Body.String(), errForbiddenTaskAction)
	recs := capture.Records()
	require.Len(t, recs, 1)
	assert.Equal(t, nswaudit.StatusFailure, recs[0].Status)
	assert.Equal(t, "step_not_guarded", recs[0].Metadata["error"])
	assert.Equal(t, "s1", recs[0].Metadata["stepId"])
}

func TestWriteCompletion_InvalidPayloadIsABadRequest(t *testing.T) {
	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/", nil)
	invalid := &payloadschema.InvalidPayloadError{Violations: []payloadschema.Violation{
		{Path: "/outcome", Message: "missing property"},
	}}

	(&HTTPHandler{}).writeCompletion(recorder, req, fmt.Errorf("PRE_RESUME extension %q failed: %w", "payload_schema", invalid))

	assert.Equal(t, http.StatusBadRequest, recorder.Code)
	assert.Contains(t, recorder.Body.String(), errInvalidPayload)
	// The failing fields are logged, not sent to the caller.
	assert.NotContains(t, recorder.Body.String(), "/outcome")
}

// --- admitSubmitter ----------------------------------------------------------

// mayAct is a CanAct that lets every caller act.
func mayAct(context.Context, store.TaskRecord, taskauthz.Input) (bool, error) { return true, nil }

// asCaller attaches p to req, with the authz input the routes' gate attaches for a user.
func asCaller(req *http.Request, p *authn.Principal) *http.Request {
	ctx := authn.ContextWithPrincipal(req.Context(), p)
	return req.WithContext(taskauthz.WithInput(ctx, taskauthz.Input{Kind: taskauthz.KindUser, UserID: p.UserID}))
}

// countingReader counts the bytes read from r, so a test can tell whether a body was read.
type countingReader struct {
	r io.Reader
	n int
}

func (c *countingReader) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	c.n += n
	return n, err
}

// The callback route names its task and step by a token, which only encodes UUIDs, so the
// admission tests use UUID ids on every route.
const (
	admitTaskID      = "6aad0417-9a6d-4407-9509-2e51d8fcae99"
	admitStepID      = "ca7ed707-1dba-43ca-94bf-10eddf00df3c"
	admitOtherStepID = "0d4f3c6e-1f52-4b1e-9a3c-6a4f0b9e2d11"
)

// admitTask is pendingFileTask under the admission tests' UUID ids.
func admitTask() *fakeTaskFetcher {
	f := pendingFileTask()
	f.record.TaskID = admitTaskID
	f.record.ActiveStepID = admitStepID
	return f
}

// callbackTask returns a callback token for admitTask's active step, and the task.
func callbackTask(t *testing.T) (string, *fakeTaskFetcher) {
	t.Helper()
	token, err := callbacktoken.Encode(admitTaskID, admitStepID)
	require.NoError(t, err)
	return token, admitTask()
}

// writeRoutes are the three routes that take a submission or a draft, each with how to
// address step stepID of task taskID on it.
var writeRoutes = []struct {
	name    string
	serve   func(h *HTTPHandler) http.HandlerFunc
	request func(t *testing.T, taskID, stepID string, body io.Reader) *http.Request
}{
	{
		name:  "complete",
		serve: func(h *HTTPHandler) http.HandlerFunc { return h.HandleCompleteTaskStep },
		request: func(_ *testing.T, taskID, stepID string, body io.Reader) *http.Request {
			req := httptest.NewRequest(http.MethodPost, "/api/v1/tasks/"+taskID+"/steps/"+stepID, body)
			req.SetPathValue("id", taskID)
			req.SetPathValue("stepId", stepID)
			return req
		},
	},
	{
		name:  "callback",
		serve: func(h *HTTPHandler) http.HandlerFunc { return h.HandleCompleteTaskStepByToken },
		request: func(t *testing.T, taskID, stepID string, body io.Reader) *http.Request {
			token, err := callbacktoken.Encode(taskID, stepID)
			require.NoError(t, err)
			req := httptest.NewRequest(http.MethodPost, "/api/v1/callbacks/"+token, body)
			req.SetPathValue("token", token)
			return req
		},
	},
	{
		name:  "draft",
		serve: func(h *HTTPHandler) http.HandlerFunc { return h.HandleSaveDraft },
		request: func(_ *testing.T, taskID, stepID string, body io.Reader) *http.Request {
			req := httptest.NewRequest(http.MethodPut, "/api/v1/tasks/"+taskID+"/steps/"+stepID+"/draft", body)
			req.SetPathValue("id", taskID)
			req.SetPathValue("stepId", stepID)
			return req
		},
	},
}

// A submission or draft that can't be taken is refused before its body is read: no
// caller identity, a task that does not exist or can't be read, a step the task has
// left, or a caller who may not act on the step.
func TestWriteRoutes_RefuseBeforeReadingTheBody(t *testing.T) {
	refuse := func(context.Context, store.TaskRecord, taskauthz.Input) (bool, error) { return false, nil }
	fail := func(context.Context, store.TaskRecord, taskauthz.Input) (bool, error) {
		return false, errors.New("artifacts unavailable")
	}
	tests := []struct {
		name       string
		fetcher    *fakeTaskFetcher
		stepID     string
		canAct     CanAct
		noCaller   bool
		wantStatus int
		wantError  string
	}{
		{"no caller identity", admitTask(), admitStepID, mayAct, true, http.StatusUnauthorized, "unauthenticated"},
		{"unknown task", &fakeTaskFetcher{}, admitStepID, mayAct, false, http.StatusNotFound, "task_not_found"},
		{"task can't be read", &fakeTaskFetcher{err: errors.New("db down")}, admitStepID, mayAct, false, http.StatusInternalServerError, "task_read_failed"},
		{"step no longer active", admitTask(), admitOtherStepID, mayAct, false, http.StatusConflict, "stale_step"},
		{"caller may not act", admitTask(), admitStepID, refuse, false, http.StatusForbidden, "forbidden"},
		{"may-act check fails", admitTask(), admitStepID, fail, false, http.StatusInternalServerError, "act_check_failed"},
		{"no may-act check", admitTask(), admitStepID, nil, false, http.StatusInternalServerError, "act_check_failed"},
	}
	for _, route := range writeRoutes {
		for _, tt := range tests {
			t.Run(route.name+"/"+tt.name, func(t *testing.T) {
				client, capture := nswaudit.NewWithCapture()
				h := &HTTPHandler{Store: tt.fetcher, CanAct: tt.canAct, MaxRequestBytes: 1024, Audit: client}
				body := &countingReader{r: strings.NewReader(`{"command":"submit","payload":{}}`)}
				req := route.request(t, admitTaskID, tt.stepID, body)
				if !tt.noCaller {
					req = asCaller(req, userPrincipal("bob"))
				}
				rec := httptest.NewRecorder()

				route.serve(h)(rec, req)

				assert.Equal(t, tt.wantStatus, rec.Code, rec.Body.String())
				assert.Zero(t, body.n, "body bytes read")
				recs := capture.Records()
				require.Len(t, recs, 1)
				assert.Equal(t, nswaudit.StatusFailure, recs[0].Status)
				assert.Equal(t, admitTaskID, recs[0].TargetID)
				assert.Equal(t, tt.wantError, recs[0].Metadata["error"])
				assert.Equal(t, tt.wantStatus, recs[0].Metadata["status"])
			})
		}
	}
}

// The may-act check decides on the input the routes' gate attached for the caller.
func TestWriteRoutes_AskCanActForTheCaller(t *testing.T) {
	for _, route := range writeRoutes {
		t.Run(route.name, func(t *testing.T) {
			var got taskauthz.Input
			var gotTask string
			canAct := func(_ context.Context, record store.TaskRecord, in taskauthz.Input) (bool, error) {
				got, gotTask = in, record.TaskID
				return false, nil
			}
			h := &HTTPHandler{Store: admitTask(), CanAct: canAct, MaxRequestBytes: 1024}
			req := route.request(t, admitTaskID, admitStepID, strings.NewReader(`{}`))
			req = req.WithContext(taskauthz.WithInput(req.Context(), taskauthz.Input{Kind: taskauthz.KindClient, ClientID: "FCAU_TO_NSW"}))

			route.serve(h)(httptest.NewRecorder(), req)

			assert.Equal(t, taskauthz.KindClient, got.Kind)
			assert.Equal(t, "FCAU_TO_NSW", got.ClientID)
			assert.Equal(t, admitTaskID, gotTask)
		})
	}
}

// Once admitted, a body over the limit, or with a field the envelope does not declare,
// is refused, and the refusal is audited.
func TestWriteRoutes_RefuseABadEnvelopeOnceAdmitted(t *testing.T) {
	tests := []struct {
		name       string
		body       string
		wantStatus int
		wantBody   string
	}{
		{"over the limit", `{"payload":{"x":"` + strings.Repeat("a", 2048) + `"}}`, http.StatusRequestEntityTooLarge, httputil.MsgBodyTooLarge},
		{"an unknown field", `{"command":"submit","payload":{},"step":"x"}`, http.StatusBadRequest, `invalid request body: unknown field \"step\"`},
	}
	for _, route := range writeRoutes {
		for _, tt := range tests {
			t.Run(route.name+"/"+tt.name, func(t *testing.T) {
				client, capture := nswaudit.NewWithCapture()
				h := &HTTPHandler{Store: admitTask(), CanAct: mayAct, MaxRequestBytes: 1024, Audit: client}
				req := asCaller(route.request(t, admitTaskID, admitStepID, strings.NewReader(tt.body)), userPrincipal("bob"))
				rec := httptest.NewRecorder()

				route.serve(h)(rec, req)

				assert.Equal(t, tt.wantStatus, rec.Code)
				assert.Contains(t, rec.Body.String(), tt.wantBody)
				recs := capture.Records()
				require.Len(t, recs, 1)
				assert.Equal(t, tt.wantStatus, recs[0].Metadata["status"])
			})
		}
	}
}

// An empty body reads as an empty envelope: a draft needs none, and a submission is
// refused only for its missing command.
func TestDecodeEnvelope_EmptyBodyIsAnEmptyEnvelope(t *testing.T) {
	h := &HTTPHandler{MaxRequestBytes: 1024}
	req := httptest.NewRequest(http.MethodPut, "/", strings.NewReader(""))

	got, ok := h.decodeEnvelope(httptest.NewRecorder(), req, "taskId", testTaskID, "stepId", "step-1")

	require.True(t, ok)
	assert.Equal(t, completeTaskStepRequest{}, got)
}
