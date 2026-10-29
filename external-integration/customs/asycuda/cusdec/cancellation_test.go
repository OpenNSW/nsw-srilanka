package cusdec

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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

// A cancellation opens the notice on the declaration's case, telling the trader
// which declaration Customs cancelled and when. Nothing else is touched: no
// step is completed, and the declaration's status stays as it was.
func TestProcessCancellation_OpensTheNotice(t *testing.T) {
	repo := &mockCusdecRepository{declsByEdgeID: map[string]*CusdecDeclaration{"edge-123": registeredDeclaration()}}
	tasks := &mockTaskCompleter{}
	flows := &recordingFlows{}
	service := NewWebhookService(repo, fakeTasks{}, tasks, flows)

	require.NoError(t, service.ProcessCancellation(context.Background(), cancellation(t)))

	require.Len(t, flows.started, 1)
	got := flows.started[0]
	assert.Equal(t, testCaseID, got.caseID)
	assert.Equal(t, CancelledFlowTemplateID, got.templateID)
	assert.Equal(t, testCaseID, got.vars["consignmentId"])
	assert.Equal(t, "CBEX1/2026/E/1047", got.vars["cusdec_number"])
	assert.Equal(t, "2026-10-20T07:00:00Z", got.vars["cancelled_at"])
	assert.False(t, repo.updateCalled, "a cancellation leaves the declaration's status as it was")
	tasks.AssertNotCalled(t, "CompleteTaskStep")
}

// SLC Edge retries a notification (§2): every delivery of the cancellation names
// the same notice, so it is opened once.
func TestProcessCancellation_Redelivery(t *testing.T) {
	repo := &mockCusdecRepository{declsByEdgeID: map[string]*CusdecDeclaration{"edge-123": registeredDeclaration()}}
	flows := &recordingFlows{}
	service := NewWebhookService(repo, fakeTasks{}, &mockTaskCompleter{}, flows)
	ctx := context.Background()

	require.NoError(t, service.ProcessCancellation(ctx, cancellation(t)))
	require.NoError(t, service.ProcessCancellation(ctx, cancellation(t)))

	require.Len(t, flows.started, 2)
	assert.Equal(t, flows.started[0].workflowID, flows.started[1].workflowID, "a redelivery names the same notice")
}

func TestProcessCancellation_Errors(t *testing.T) {
	unlinked := registeredDeclaration()
	unlinked.CaseID = ""
	for name, tc := range map[string]struct {
		decls map[string]*CusdecDeclaration
		want  error
	}{
		"reference not registered": {decls: map[string]*CusdecDeclaration{}, want: ErrCusdecNotFoundByRef},
		"no case recorded":         {decls: map[string]*CusdecDeclaration{"edge-123": unlinked}, want: ErrCaseNotRecorded},
	} {
		t.Run(name, func(t *testing.T) {
			flows := &recordingFlows{}
			service := NewWebhookService(&mockCusdecRepository{declsByEdgeID: tc.decls}, fakeTasks{}, &mockTaskCompleter{}, flows)

			require.ErrorIs(t, service.ProcessCancellation(context.Background(), cancellation(t)), tc.want)
			assert.Empty(t, flows.started)
		})
	}
}
