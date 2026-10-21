package tasks

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/OpenNSW/core/artifact"
	"github.com/OpenNSW/core/artifact/adapter/generictemplate"
	"github.com/OpenNSW/core/artifact/adapter/steptemplate"
	"github.com/OpenNSW/core/artifact/testutil"
	"github.com/OpenNSW/core/taskflow/extensions"
	"github.com/OpenNSW/core/taskflow/orchestrator"
	"github.com/OpenNSW/core/taskflow/store"
	workflow "github.com/OpenNSW/core/workflow"
	"github.com/OpenNSW/nsw-srilanka/internal/authn"
	"github.com/OpenNSW/nsw-srilanka/internal/tasks/extensions/payloadschema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// memTaskStore is a task store holding one task, enough for a step to be
// submitted or saved as a draft through core's task manager.
type memTaskStore struct {
	mu     sync.Mutex
	record store.TaskRecord
}

func (s *memTaskStore) GetTask(_ context.Context, taskID string) (store.TaskRecord, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if taskID != s.record.TaskID {
		return store.TaskRecord{}, false
	}
	return s.record.DeepCopy(), true
}

func (s *memTaskStore) WriteDraft(_ context.Context, taskID, stepID string, seq int64, data map[string]any) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if taskID != s.record.TaskID || stepID != s.record.ActiveStepID || seq != s.record.Seq {
		return 0, nil
	}
	s.record.Data = data
	return 1, nil
}

func (s *memTaskStore) PersistSubmission(_ context.Context, taskID, stepID string, seq int64, data map[string]any) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if taskID != s.record.TaskID || stepID != s.record.ActiveStepID || seq != s.record.Seq {
		return 0, nil
	}
	s.record.Data, s.record.State, s.record.Seq = data, store.StateAdvancing, seq+1
	return 1, nil
}

// The task manager calls none of these on a submission or a draft.
func (s *memTaskStore) InitTask(context.Context, store.TaskRecord) error { panic("not used") }
func (s *memTaskStore) GetTaskByWorkflowID(context.Context, string) (store.TaskRecord, bool) {
	panic("not used")
}
func (s *memTaskStore) GetAllTasks(context.Context, string) []store.TaskRecord { panic("not used") }
func (s *memTaskStore) ClaimStep(context.Context, string, store.StepClaim) (int64, error) {
	panic("not used")
}
func (s *memTaskStore) WriteRenderState(context.Context, string, string, int64, string, map[string]any) (int64, error) {
	panic("not used")
}
func (s *memTaskStore) CompleteTask(context.Context, string, int64) (int64, error) {
	panic("not used")
}
func (s *memTaskStore) ClaimTask(context.Context, string, string, time.Time) (int64, error) {
	panic("not used")
}
func (s *memTaskStore) ReleaseTask(context.Context, string, string) (int64, error) {
	panic("not used")
}

// resumedWorkflow stands in for the task's workflow, counting the steps
// completed in it. Only CompleteActivation is called on a submission.
type resumedWorkflow struct {
	workflow.TemporalManager
	completed int
}

func (w *resumedWorkflow) CompleteActivation(context.Context, string, string, string, map[string]any) error {
	w.completed++
	return nil
}

// draftValidationFixture is a task on a step whose template enables drafts and
// the payload schema extension, over a form that requires "name". The handler
// runs core's task manager with the extension registered as the app registers
// it.
func draftValidationFixture(t *testing.T) (*HTTPHandler, *memTaskStore, *resumedWorkflow) {
	t.Helper()
	reg := artifact.NewRegistry(testutil.MemLoader{
		"step.json": []byte(`{
			"id": "form-step",
			"task_type": "USER_INPUT",
			"output_namespace": "form",
			"draft": {"command": "save_as_draft"},
			"extensions": [{"id": "payload_schema", "phase": "PRE_RESUME", "properties": {"form": "the-form"}}]
		}`),
		"form.json": []byte(`{"schema": {
			"type": "object",
			"properties": {"name": {"type": "string", "minLength": 1}},
			"required": ["name"]
		}}`),
	})
	reg.RegisterArtifact("form-step", steptemplate.Kind, "", "step.json")
	reg.RegisterArtifact("the-form", generictemplate.Kind, "", "form.json")

	extReg := extensions.NewRegistry()
	formOf := func(ctx context.Context, id string) ([]byte, error) {
		raw, err := generictemplate.Load(ctx, reg, id)
		return []byte(raw), err
	}
	require.NoError(t, payloadschema.Register(extReg, formOf))

	db := &memTaskStore{record: store.TaskRecord{
		TaskID: testTaskID, TaskWorkflowID: "task-wf", ActiveStepID: "step-1", ActiveTaskTemplateID: "form-step",
		Seq: 4, State: "PENDING_USER", Data: map[string]any{"form": map[string]any{}},
	}}
	wf := &resumedWorkflow{}
	h := &HTTPHandler{
		Manager:         orchestrator.NewTaskManager(db, reg, nil, extReg, wf, nil, nil),
		Store:           db,
		Files:           NewFileBinding(newFakeFileRefs(), nil),
		MaxRequestBytes: 1024,
	}
	return h, db, wf
}

// callStep sends body to step-1 of the fixture's task through serve, as bob.
func callStep(method, path, body string, serve http.HandlerFunc) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.SetPathValue("id", testTaskID)
	req.SetPathValue("stepId", "step-1")
	req = req.WithContext(authn.ContextWithPrincipal(req.Context(), userPrincipal("bob")))
	recorder := httptest.NewRecorder()
	serve(recorder, req)
	return recorder
}

// An incomplete draft is saved: the draft route marks the call as a draft, so
// the form's rules are not applied to it.
func TestHandleSaveDraft_SavesAnIncompleteDraft(t *testing.T) {
	h, db, wf := draftValidationFixture(t)

	recorder := callStep(http.MethodPut, "/api/v1/tasks/"+testTaskID+"/steps/step-1/draft", `{"payload":{"note":"half done"}}`, h.HandleSaveDraft)

	require.Equal(t, http.StatusNoContent, recorder.Code, recorder.Body.String())
	assert.Equal(t, map[string]any{"note": "half done"}, db.record.Data["form"])
	assert.Zero(t, wf.completed, "a draft must not complete the step")
}

// A submission is validated even when it names the step's draft command: only
// the draft route makes a draft, not the command a caller sends.
func TestHandleCompleteTaskStep_ValidatesASubmissionNamingTheDraftCommand(t *testing.T) {
	h, db, wf := draftValidationFixture(t)

	recorder := callStep(http.MethodPost, "/api/v1/tasks/"+testTaskID+"/steps/step-1", `{"command":"save_as_draft","payload":{"note":"half done"}}`, h.HandleCompleteTaskStep)

	require.Equal(t, http.StatusBadRequest, recorder.Code, recorder.Body.String())
	assert.Contains(t, recorder.Body.String(), errInvalidPayload)
	assert.Equal(t, map[string]any{}, db.record.Data["form"], "a rejected submission must not be stored")
	assert.Zero(t, wf.completed, "a rejected submission must not complete the step")
}

// A submission that matches the form completes the step, so the rejection
// above comes from the form's rules and not from the setup.
func TestHandleCompleteTaskStep_AcceptsASubmissionMatchingTheForm(t *testing.T) {
	h, db, wf := draftValidationFixture(t)

	recorder := callStep(http.MethodPost, "/api/v1/tasks/"+testTaskID+"/steps/step-1", `{"command":"submit","payload":{"name":"Alice"}}`, h.HandleCompleteTaskStep)

	require.Equal(t, http.StatusNoContent, recorder.Code, recorder.Body.String())
	assert.Equal(t, map[string]any{"name": "Alice"}, db.record.Data["form"])
	assert.Equal(t, 1, wf.completed)
}
