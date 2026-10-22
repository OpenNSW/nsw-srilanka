package cusdec

import (
	"context"
	"testing"
	"time"

	"github.com/OpenNSW/core/taskflow/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// The export flow the tests' declaration was dispatched under: its customs work
// ran in a branch, testWorkflowID.
const exportFlowID = "case-1:trade-export-v1:1"

// fakeWorkflows resolves workflows to the flow they run under and records what
// it terminates.
type fakeWorkflows struct {
	roots      map[string]string
	terminated []string
}

func (f *fakeWorkflows) RootWorkflowID(_ context.Context, workflowID string) (string, error) {
	if root, ok := f.roots[workflowID]; ok {
		return root, nil
	}
	return workflowID, nil
}

func (f *fakeWorkflows) TerminateWorkflow(_ context.Context, workflowID, _ string) error {
	f.terminated = append(f.terminated, workflowID)
	return nil
}

func cancellation(t *testing.T) CusdecCancelledRequest {
	t.Helper()
	req := CusdecCancelledRequest{
		EventType:   EventCusdecCancelled,
		ProcessedAt: time.Date(2026, 10, 20, 7, 0, 0, 0, time.UTC),
		Payload:     cancelledPayload{CusdecRef: amendedRef},
	}
	require.NoError(t, req.Validate())
	return req
}

// caseTasks is the tests' case: the export flow's payment wait still open in its
// customs branch, a task of it already completed, and an open amendment notice,
// a flow of its own.
func caseTasks() fakeTasks {
	return fakeTasks{testCaseID: {
		{TaskID: "payment-wait", ParentWorkflowID: testWorkflowID, CaseID: testCaseID, State: "QUEUED_EXTERNALLY"},
		{TaskID: "cusdec-review", ParentWorkflowID: testWorkflowID, CaseID: testCaseID, State: store.StateCompleted},
		{TaskID: "amendment", ParentWorkflowID: "case-1:customs-cusdec-amended:x", CaseID: testCaseID, State: "PENDING_USER"},
	}}
}

// A cancellation ends the export flow the declaration was dispatched under, with
// its open tasks, records the flow as ended, opens the notice and marks the
// declaration cancelled. A flow of the case it does not belong to is left alone.
func TestProcessCancellation_EndsTheExportFlow(t *testing.T) {
	repo := &mockCusdecRepository{declsByEdgeID: map[string]*CusdecDeclaration{"edge-123": registeredDeclaration()}}
	tm := &mockTaskCompleter{}
	tm.On("CancelTask", mock.Anything, "payment-wait", "Customs cancelled declaration CBEX1/2026/E/1047").Return(nil)
	flows := &recordingFlows{}
	wf := &fakeWorkflows{roots: map[string]string{testWorkflowID: exportFlowID}}
	service := NewWebhookService(repo, caseTasks(), tm, flows, wf)

	require.NoError(t, service.ProcessCancellation(context.Background(), cancellation(t)))

	assert.Equal(t, []string{exportFlowID}, wf.terminated)
	tm.AssertExpectations(t)
	tm.AssertNotCalled(t, "CancelTask", mock.Anything, "amendment", mock.Anything)
	tm.AssertNotCalled(t, "CancelTask", mock.Anything, "cusdec-review", mock.Anything)
	assert.Equal(t, []string{exportFlowID}, flows.ended)
	require.Len(t, flows.started, 1)
	assert.Equal(t, CancelledFlowTemplateID, flows.started[0].templateID)
	assert.Equal(t, "CBEX1/2026/E/1047", flows.started[0].vars["cusdec_number"])
	assert.Equal(t, "2026-10-20T07:00:00Z", flows.started[0].vars["cancelled_at"])
	assert.Equal(t, CusdecStatusCancelled, repo.updatedDecl.Status)
}

// Once applied, a redelivery is acknowledged without ending or opening anything.
func TestProcessCancellation_Redelivery(t *testing.T) {
	decl := registeredDeclaration()
	decl.Status = CusdecStatusCancelled
	flows, wf := &recordingFlows{}, &fakeWorkflows{}
	service := NewWebhookService(&mockCusdecRepository{declsByEdgeID: map[string]*CusdecDeclaration{"edge-123": decl}},
		caseTasks(), &mockTaskCompleter{}, flows, wf)

	require.ErrorIs(t, service.ProcessCancellation(context.Background(), cancellation(t)), ErrDuplicateEvent)
	assert.Empty(t, wf.terminated)
	assert.Empty(t, flows.started)
}

// An event for a cancelled declaration has nothing left to advance, and must not
// move its status back.
func TestProcessEvent_AfterCancellation(t *testing.T) {
	decl := registeredDeclaration()
	decl.Status = CusdecStatusCancelled
	repo := &mockCusdecRepository{declsByEdgeID: map[string]*CusdecDeclaration{"edge-123": decl}}
	service := NewWebhookService(repo, caseTasks(), &mockTaskCompleter{}, nil, nil)

	err := service.ProcessEvent(context.Background(), CusdecEventRequest{
		Event: "PAYMENT_CONFIRMED", ProcessAt: time.Now(), Payload: cusdecEventPayload{CusdecRef: amendedRef},
	})

	require.ErrorIs(t, err, ErrDuplicateEvent)
	assert.False(t, repo.updateCalled)
	assert.Equal(t, CusdecStatusCancelled, decl.Status)
}

func TestProcessCancellation_Errors(t *testing.T) {
	unlinked := registeredDeclaration()
	unlinked.WorkflowID = ""
	for name, tc := range map[string]struct {
		decls map[string]*CusdecDeclaration
		want  error
	}{
		"reference not registered": {decls: map[string]*CusdecDeclaration{}, want: ErrCusdecNotFoundByRef},
		"no workflow recorded":     {decls: map[string]*CusdecDeclaration{"edge-123": unlinked}, want: ErrCaseNotRecorded},
	} {
		t.Run(name, func(t *testing.T) {
			wf := &fakeWorkflows{}
			service := NewWebhookService(&mockCusdecRepository{declsByEdgeID: tc.decls}, caseTasks(), &mockTaskCompleter{}, &recordingFlows{}, wf)

			require.ErrorIs(t, service.ProcessCancellation(context.Background(), cancellation(t)), tc.want)
			assert.Empty(t, wf.terminated)
		})
	}
}
