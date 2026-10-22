package plugins

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/OpenNSW/core/remote"
	"github.com/OpenNSW/core/taskflow/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// recordingInterpreter accepts or rejects every call as told, and records the
// dispatches it is asked to remember.
type recordingInterpreter struct {
	accept    bool
	recordErr error
	recorded  []recordedDispatch
}

type recordedDispatch struct {
	taskID, caseID, workflowID string
	captured                   map[string]any
}

func (r *recordingInterpreter) BuildRequest(map[string]any) remote.Body {
	return remote.JSONBody{V: map[string]any{}}
}

func (r *recordingInterpreter) Interpret(error, map[string]any) (bool, map[string]any) {
	return r.accept, map[string]any{"edgeId": "edge-1"}
}

func (r *recordingInterpreter) RecordDispatch(_ context.Context, taskID, caseID, workflowID string, captured map[string]any) error {
	r.recorded = append(r.recorded, recordedDispatch{taskID, caseID, workflowID, captured})
	return r.recordErr
}

func dispatchContext() pluginContext {
	return pluginContext{
		Context:         context.Background(),
		Record:          &store.TaskRecord{TaskID: "task-1", CaseID: "case-1", ParentWorkflowID: "wf-1", Data: map[string]any{}},
		Inputs:          map[string]any{},
		OutputNamespace: "cig",
	}
}

const dispatchConfig = `{"service_id":"cda","path":"/review"}`

// An accepted call is recorded against the task that made it, with what the
// interpreter captured, so the callback that answers it can find the task.
func TestAPICallPlugin_RecordsAnAcceptedDispatch(t *testing.T) {
	m, _ := newOGA(t)
	interp := &recordingInterpreter{accept: true}

	require.NoError(t, NewAPICallPluginWithInterpreter(m, interp).Execute(dispatchContext(), json.RawMessage(dispatchConfig)))

	require.Len(t, interp.recorded, 1)
	assert.Equal(t, recordedDispatch{"task-1", "case-1", "wf-1", map[string]any{"edgeId": "edge-1"}}, interp.recorded[0])
}

// A rejected call is never answered by a callback, so nothing is recorded.
func TestAPICallPlugin_DoesNotRecordARejectedDispatch(t *testing.T) {
	m, _ := newOGA(t)
	interp := &recordingInterpreter{accept: false}

	require.NoError(t, NewAPICallPluginWithInterpreter(m, interp).Execute(dispatchContext(), json.RawMessage(dispatchConfig)))

	assert.Empty(t, interp.recorded)
}

// A dispatch that could not be recorded fails the step: nothing could find the
// task when its answer arrives.
func TestAPICallPlugin_FailsWhenTheDispatchCannotBeRecorded(t *testing.T) {
	m, _ := newOGA(t)
	interp := &recordingInterpreter{accept: true, recordErr: errors.New("db down")}

	err := NewAPICallPluginWithInterpreter(m, interp).Execute(dispatchContext(), json.RawMessage(dispatchConfig))

	require.ErrorContains(t, err, "db down")
}
