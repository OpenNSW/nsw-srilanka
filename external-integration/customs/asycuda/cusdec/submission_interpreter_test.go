package cusdec

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// An accepted declaration is recorded against the workflow that sent it, by the
// edgeId SLC Edge acknowledged it with, so its integration result and events can
// find that workflow's parked tasks.
func TestCusdecInterpreter_RecordDispatch(t *testing.T) {
	repo := &mockCusdecRepository{declsByEdgeID: map[string]*CusdecDeclaration{}}

	require.NoError(t, NewCusdecInterpreter(nil, repo).RecordDispatch(context.Background(), "task-1", "case-1", "wf-1",
		map[string]any{"edgeId": "edge-9", "status": "RECEIVED"}))

	assert.Equal(t, &CusdecDeclaration{EdgeID: "edge-9", Status: CusdecStatusSubmitted, CaseID: "case-1", WorkflowID: "wf-1"}, repo.declsByEdgeID["edge-9"])
	assert.Error(t, NewCusdecInterpreter(nil, repo).RecordDispatch(context.Background(), "task-1", "case-1", "wf-1", map[string]any{}),
		"an acceptance without an edgeId could never be answered")
}
