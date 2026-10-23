package cdn

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var cancelledRef = DocumentReference{Office: "CBEX1", Year: "2026", Serial: "C", Number: 28237}

func cancellation(t *testing.T) CDNCancelledRequest {
	t.Helper()
	req := CDNCancelledRequest{
		EventType:   EventCDNCancelled,
		ProcessedAt: time.Date(2026, 10, 21, 7, 0, 0, 0, time.UTC),
		Payload:     cancelledPayload{CDNRef: cancelledRef},
	}
	require.NoError(t, req.Validate())
	return req
}

// noteIn is a registered note of wf-branch-0 of case-1 in the given status,
// under the reference the cancellation names (the mock keys on
// year-office-serial).
func noteIn(status DispatchNoteStatus) map[string]*DispatchNote {
	return map[string]*DispatchNote{
		"2026-CBEX1-C": {
			ID: "note-1", EdgeID: "edge-1", Status: status, CaseID: "case-1", WorkflowID: "wf-branch-0",
			CDNOffice: "CBEX1", CDNYear: "2026", CDNSerial: "C", CDNNumber: 28237,
		},
	}
}

// parkedAcknowledgmentWait is case-1 with wf-branch-0 waiting on its note's
// acknowledgment, beside another branch waiting on its own, which must be left
// waiting.
func parkedAcknowledgmentWait() fakeTasks {
	return fakeTasks{"case-1": {
		{TaskID: "wait_ack:other", ActiveStepID: "step-other", ParentWorkflowID: "wf-branch-1", CaseID: "case-1",
			ActiveTaskTemplateID: acknowledgmentWaitTemplateID, State: "QUEUED_EXTERNALLY"},
		{TaskID: "wait_ack:abc", ActiveStepID: "step-abc", ParentWorkflowID: "wf-branch-0", CaseID: "case-1",
			ActiveTaskTemplateID: acknowledgmentWaitTemplateID, State: "QUEUED_EXTERNALLY"},
	}}
}

// A cancellation marks the note cancelled and releases its branch's
// acknowledgment wait as cancelled, so the branch finishes. The consignment's
// other branches keep waiting on their own notes.
func TestProcessCancellation_EndsTheBranchWait(t *testing.T) {
	repo := &mockRepository{byCDNRef: noteIn(DispatchNoteStatusIntegrated)}
	recorder := &resumeRecorder{}
	svc := NewCDNWebhookService(repo, parkedAcknowledgmentWait(), recorder, nil)

	require.NoError(t, svc.ProcessCancellation(context.Background(), cancellation(t)))

	require.NotNil(t, repo.updated)
	assert.Equal(t, DispatchNoteStatusCancelled, repo.updated.Status)
	payload, ok := recorder.completed["wait_ack:abc"]
	require.True(t, ok, "the branch's acknowledgment wait was not released; it would wait forever")
	assert.Equal(t, "step-abc", recorder.steps["wait_ack:abc"])
	assert.Equal(t, "submit", payload["__command"])
	assert.Equal(t, "CANCELLED", payload["ack_status"])
	assert.NotContains(t, recorder.completed, "wait_ack:other", "another branch's wait must not be released")
}

// SLC Edge retries a notification (§2). A redelivery leaves the note as it is
// and releases the wait again, in case the first delivery found it not yet
// parked.
func TestProcessCancellation_RedeliveryReleasesTheWaitAgain(t *testing.T) {
	repo := &mockRepository{byCDNRef: noteIn(DispatchNoteStatusCancelled)}
	recorder := &resumeRecorder{}
	svc := NewCDNWebhookService(repo, parkedAcknowledgmentWait(), recorder, nil)

	require.NoError(t, svc.ProcessCancellation(context.Background(), cancellation(t)))

	assert.Nil(t, repo.updated)
	assert.Equal(t, "CANCELLED", recorder.completed["wait_ack:abc"]["ack_status"])
}

// A note cancelled after its acknowledgment has no wait left: its branch has
// finished. It is still recorded as cancelled.
func TestProcessCancellation_AfterAcknowledgment(t *testing.T) {
	repo := &mockRepository{byCDNRef: noteIn(DispatchNoteStatusAcknowledged)}
	recorder := &resumeRecorder{}
	svc := NewCDNWebhookService(repo, parkedAcknowledgmentWait(), recorder, nil)

	require.NoError(t, svc.ProcessCancellation(context.Background(), cancellation(t)))

	require.NotNil(t, repo.updated)
	assert.Equal(t, DispatchNoteStatusCancelled, repo.updated.Status)
	assert.Empty(t, recorder.completed)
}

func TestProcessCancellation_UnknownReference(t *testing.T) {
	svc := NewCDNWebhookService(&mockRepository{}, nil, nil, nil)

	err := svc.ProcessCancellation(context.Background(), cancellation(t))

	assert.ErrorIs(t, err, ErrDispatchNoteNotFoundByCDNRef)
}

func TestProcessCancellation_InvalidState(t *testing.T) {
	repo := &mockRepository{byCDNRef: noteIn(DispatchNoteStatusFailed)}
	svc := NewCDNWebhookService(repo, nil, nil, nil)

	err := svc.ProcessCancellation(context.Background(), cancellation(t))

	require.Error(t, err)
	assert.Contains(t, err.Error(), "cannot cancel dispatch note in status FAILED")
	assert.Nil(t, repo.updated)
}

// An acknowledgment for a cancelled note is acknowledged without action: the
// note stays cancelled and its finished branch is not touched.
func TestProcessAcknowledgment_CancelledNote(t *testing.T) {
	repo := &mockRepository{byCDNRef: noteIn(DispatchNoteStatusCancelled)}
	recorder := &resumeRecorder{}
	svc := NewCDNWebhookService(repo, parkedAcknowledgmentWait(), recorder, nil)

	req := CDNAcknowledgmentRequest{Event: "CDN_ACKNOWLEDGED", Payload: acknowledgmentPayload{CDNRef: cancelledRef}}
	require.NoError(t, svc.ProcessAcknowledgment(context.Background(), req))

	assert.Nil(t, repo.updated)
	assert.Empty(t, recorder.completed)
}

// A replayed integration result for a cancelled note does not record it as
// integrated again.
func TestProcessIntegrationResult_CancelledNoteStaysCancelled(t *testing.T) {
	repo := &mockRepository{byEdgeID: noteIn(DispatchNoteStatusCancelled)}
	repo.byEdgeID["edge-1"] = repo.byEdgeID["2026-CBEX1-C"]
	svc := NewCDNWebhookService(repo, nil, nil, nil)

	req := CDNIntegrationResultRequest{
		Event:   "CDN_INTEGRATED",
		Payload: integrationResultPayload{EdgeID: "edge-1", Integrated: true, CDNRef: cancelledRef},
	}
	require.NoError(t, svc.ProcessIntegrationResult(context.Background(), req))

	assert.Nil(t, repo.updated)
	assert.Nil(t, repo.created)
	assert.Equal(t, DispatchNoteStatusCancelled, repo.byEdgeID["edge-1"].Status)
}

// The wait is released before the note is marked cancelled. A cancellation whose
// wait has not parked yet is retried, and leaves the note INTEGRATED, so the
// retry finds the wait and releases it.
func TestProcessCancellation_WaitNotParkedYetIsRetried(t *testing.T) {
	repo := &mockRepository{byCDNRef: noteIn(DispatchNoteStatusIntegrated)}
	recorder := &resumeRecorder{}

	early := NewCDNWebhookService(repo, fakeTasks{}, recorder, nil)
	require.ErrorIs(t, early.ProcessCancellation(context.Background(), cancellation(t)), ErrTaskNotParked)
	assert.Nil(t, repo.updated, "the note must not be cancelled, or the retry is a redelivery")
	assert.Equal(t, DispatchNoteStatusIntegrated, repo.byCDNRef["2026-CBEX1-C"].Status)

	retry := NewCDNWebhookService(repo, parkedAcknowledgmentWait(), recorder, nil)
	require.NoError(t, retry.ProcessCancellation(context.Background(), cancellation(t)))
	assert.Equal(t, "CANCELLED", recorder.completed["wait_ack:abc"]["ack_status"])
	require.NotNil(t, repo.updated)
	assert.Equal(t, DispatchNoteStatusCancelled, repo.updated.Status)
}

// Tasks that could not be read are an error, and leave the note as it was.
func TestProcessCancellation_TaskStoreFailureLeavesTheNote(t *testing.T) {
	repo := &mockRepository{byCDNRef: noteIn(DispatchNoteStatusIntegrated)}
	recorder := &resumeRecorder{}
	svc := NewCDNWebhookService(repo, failingTasks{}, recorder, nil)

	err := svc.ProcessCancellation(context.Background(), cancellation(t))

	require.ErrorIs(t, err, errTaskStore)
	assert.Nil(t, repo.updated)
	assert.Empty(t, recorder.completed)
}
