package cdn

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/OpenNSW/core/taskflow/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockRepository struct {
	byEdgeID map[string]*DispatchNote
	byCDNRef map[string]*DispatchNote
	updated  *DispatchNote
	created  *DispatchNote
}

func (m *mockRepository) GetByEdgeID(ctx context.Context, edgeID string) (*DispatchNote, error) {
	return m.byEdgeID[edgeID], nil
}

func (m *mockRepository) GetByCDNRef(ctx context.Context, ref DocumentReference) (*DispatchNote, error) {
	key := ref.Year + "-" + ref.Office + "-" + ref.Serial
	return m.byCDNRef[key], nil
}

func (m *mockRepository) Create(ctx context.Context, note *DispatchNote) error {
	m.created = note
	return nil
}

func (m *mockRepository) Update(ctx context.Context, note *DispatchNote) error {
	m.updated = note
	return nil
}

func (m *mockRepository) RecordSubmitted(ctx context.Context, edgeID, caseID, workflowID string) error {
	m.byEdgeID[edgeID] = &DispatchNote{EdgeID: edgeID, Status: DispatchNoteStatusSubmitted, CaseID: caseID, WorkflowID: workflowID}
	return nil
}

func TestProcessIntegrationResult_Success(t *testing.T) {
	repo := &mockRepository{
		byEdgeID: map[string]*DispatchNote{
			"edge-123": {ID: "1", EdgeID: "edge-123", Status: DispatchNoteStatusSubmitted},
		},
	}
	svc := NewCDNWebhookService(repo, nil, nil, nil)

	req := CDNIntegrationResultRequest{
		Event: "INTEGRATION_RESULT",
		Payload: integrationResultPayload{
			EdgeID:     "edge-123",
			Integrated: true,
			CDNRef:     DocumentReference{Year: "2026", Office: "COL", Serial: "C", Number: 4567},
		},
	}

	err := svc.ProcessIntegrationResult(context.Background(), req)
	require.NoError(t, err)
	assert.Equal(t, DispatchNoteStatusIntegrated, repo.updated.Status)
	assert.Equal(t, "COL", repo.updated.CDNOffice)
}

func TestProcessIntegrationResult_FailureWithErrorPersistence(t *testing.T) {
	repo := &mockRepository{
		byEdgeID: map[string]*DispatchNote{
			"edge-123": {ID: "1", EdgeID: "edge-123", Status: DispatchNoteStatusSubmitted},
		},
	}
	svc := NewCDNWebhookService(repo, nil, nil, nil)

	rawErrors := json.RawMessage(`{"code":"ERR_VAL_01","message":"Invalid weight value"}`)
	req := CDNIntegrationResultRequest{
		Event: "INTEGRATION_RESULT",
		Payload: integrationResultPayload{
			EdgeID:     "edge-123",
			Integrated: false,
			Errors:     rawErrors,
		},
	}

	err := svc.ProcessIntegrationResult(context.Background(), req)
	require.NoError(t, err)
	assert.Equal(t, DispatchNoteStatusFailed, repo.updated.Status)
	assert.JSONEq(t, string(rawErrors), string(repo.updated.Errors))
}

// An unseen edgeId is the normal case, not an error: the §7.1 submission is
// recorded in the workflow rather than in this table, so the integration result
// is the first this service hears of a dispatch note.
// The dispatch step records a note before SLC Edge can call back, so a result
// for an edgeId with no row is early: it is retried, and no note is created
// without the workflow it belongs to.
func TestProcessIntegrationResult_UnrecordedEdgeIDIsRetried(t *testing.T) {
	repo := &mockRepository{byEdgeID: map[string]*DispatchNote{}}
	svc := NewCDNWebhookService(repo, nil, nil, nil)

	req := CDNIntegrationResultRequest{
		Payload: integrationResultPayload{
			EdgeID:     "edge-unseen",
			Integrated: true,
			CDNRef:     DocumentReference{Year: "2026", Office: "CBEX1", Serial: "C", Number: 28237},
		},
	}
	err := svc.ProcessIntegrationResult(context.Background(), req)

	require.ErrorIs(t, err, ErrDispatchNoteNotRecorded)
	assert.Nil(t, repo.created)
	assert.Nil(t, repo.updated)
}

func TestProcessAcknowledgment_Success(t *testing.T) {
	repo := &mockRepository{
		byCDNRef: map[string]*DispatchNote{
			"2026-COL-C": {ID: "note-123", Status: DispatchNoteStatusIntegrated},
		},
	}
	svc := NewCDNWebhookService(repo, nil, nil, nil)

	req := CDNAcknowledgmentRequest{
		Event: "ACKNOWLEDGMENT",
		Payload: acknowledgmentPayload{
			CDNRef: DocumentReference{Year: "2026", Office: "COL", Serial: "C", Number: 4567},
		},
	}

	err := svc.ProcessAcknowledgment(context.Background(), req)
	require.NoError(t, err)
	assert.Equal(t, DispatchNoteStatusAcknowledged, repo.updated.Status)
}

func TestProcessAcknowledgment_InvalidState(t *testing.T) {
	repo := &mockRepository{
		byCDNRef: map[string]*DispatchNote{
			"2026-COL-C": {ID: "note-123", Status: DispatchNoteStatusSubmitted},
		},
	}
	svc := NewCDNWebhookService(repo, nil, nil, nil)

	req := CDNAcknowledgmentRequest{
		Event: "ACKNOWLEDGMENT",
		Payload: acknowledgmentPayload{
			CDNRef: DocumentReference{Year: "2026", Office: "COL", Serial: "C", Number: 4567},
		},
	}

	err := svc.ProcessAcknowledgment(context.Background(), req)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cannot acknowledge dispatch note in status SUBMITTED")
}

// setupTestDB builds a gorm handle over a stubbed Postgres connection, matching
// the pattern used by the service tests elsewhere in this repo.
// fakeTasks stands in for core's task store: the tasks of each case.
type fakeTasks map[string][]store.TaskRecord

func (f fakeTasks) TasksOfCase(_ context.Context, caseID string) ([]store.TaskRecord, error) {
	return f[caseID], nil
}

// failingTasks is a task store whose load fails.
type failingTasks struct{}

func (failingTasks) TasksOfCase(context.Context, string) ([]store.TaskRecord, error) {
	return nil, errTaskStore
}

var errTaskStore = errors.New("task store: connection refused")

// dispatchedNote is a dispatch note as its dispatch left it: SLC Edge accepted
// it as edgeID from workflow wf-branch-0 of case case-1.
func dispatchedNote(edgeID string) *DispatchNote {
	return &DispatchNote{ID: "note-" + edgeID, EdgeID: edgeID, Status: DispatchNoteStatusSubmitted, CaseID: "case-1", WorkflowID: "wf-branch-0"}
}

// parkedIntegrationWait is case-1 with wf-branch-0's CDN task parked on the
// integration wait, beside another branch's task parked on the same template,
// which must not be resumed.
func parkedIntegrationWait() fakeTasks {
	return fakeTasks{"case-1": {
		{TaskID: "create_cdn:other", ActiveStepID: "step-other", ParentWorkflowID: "wf-branch-1", CaseID: "case-1",
			ActiveTaskTemplateID: integrationWaitTemplateID, State: "QUEUED_EXTERNALLY"},
		{TaskID: "create_cdn:abc", ActiveStepID: "step-abc", ParentWorkflowID: "wf-branch-0", CaseID: "case-1",
			ActiveTaskTemplateID: integrationWaitTemplateID, State: "QUEUED_EXTERNALLY"},
	}}
}

// resumeRecorder captures the task steps a callback completes.
type resumeRecorder struct {
	completed map[string]map[string]any
	steps     map[string]string
}

func (r *resumeRecorder) CompleteTaskStep(_ context.Context, taskID, stepID string, payload map[string]any) error {
	if r.completed == nil {
		r.completed = map[string]map[string]any{}
		r.steps = map[string]string{}
	}
	r.completed[taskID] = payload
	r.steps[taskID] = stepID
	return nil
}

// A redelivery of an already-recorded result must still release a task parked on
// it. §2 has SLC Edge retry up to four times, and the record and the workflow
// advance separately — so the second delivery is often the one that finds the
// task actually parked. Returning early there strands the trader forever.
func TestProcessIntegrationResult_RedeliveryStillResumesParkedTask(t *testing.T) {
	repo := &mockRepository{
		byEdgeID: map[string]*DispatchNote{
			"edge-1": {
				ID: "1", EdgeID: "edge-1", Status: DispatchNoteStatusIntegrated,
				CDNOffice: "CBEX1", CDNYear: "2026", CDNSerial: "C", CDNNumber: 28237,
			},
		},
	}
	// No db/taskManager wiring here: the assertion is that the call reaches the
	// resume path instead of returning at the duplicate guard, and stays a 200.
	svc := NewCDNWebhookService(repo, nil, nil, nil)

	req := CDNIntegrationResultRequest{
		Event: "CDN_INTEGRATED",
		Payload: integrationResultPayload{
			EdgeID:     "edge-1",
			Integrated: true,
			CDNRef:     DocumentReference{Office: "CBEX1", Year: "2026", Serial: "C", Number: 28240},
		},
	}
	require.NoError(t, svc.ProcessIntegrationResult(context.Background(), req))

	// The stored note is untouched by the replay.
	assert.Nil(t, repo.created)
	assert.Nil(t, repo.updated)
}

// The row is the authority once written: a replay carrying a different cdnRef
// must not hand the workflow a reference the acknowledgment can never match.
func TestStoredResult_PrefersTheRecordedReference(t *testing.T) {
	note := &DispatchNote{
		Status:    DispatchNoteStatusIntegrated,
		CDNOffice: "CBEX1", CDNYear: "2026", CDNSerial: "C", CDNNumber: 28237,
	}
	req := CDNIntegrationResultRequest{
		Payload: integrationResultPayload{
			Integrated: false,
			CDNRef:     DocumentReference{Office: "XXXXX", Year: "1999", Serial: "Z", Number: 28240},
		},
	}

	out := storedResult(req, note)
	assert.True(t, out.Payload.Integrated, "a stored INTEGRATED note is integrated regardless of the replayed flag")
	assert.Equal(t, 28237, out.Payload.CDNRef.Number)
	assert.Equal(t, "CBEX1", out.Payload.CDNRef.Office)
}

// The whole point of the §7.2 callback is that it releases the task parked on the
// dispatch note: the note names the workflow its dispatch was made from, and the
// task that workflow parked on the integration wait is completed.
func TestProcessIntegrationResult_ResumesTheParkedIntegrationWait(t *testing.T) {
	recorder := &resumeRecorder{}
	repo := &mockRepository{byEdgeID: map[string]*DispatchNote{"edge-1": dispatchedNote("edge-1")}}
	svc := NewCDNWebhookService(repo, parkedIntegrationWait(), recorder, nil)

	req := CDNIntegrationResultRequest{
		Event: "CDN_INTEGRATED",
		Payload: integrationResultPayload{
			EdgeID:     "edge-1",
			Integrated: true,
			CDNRef:     DocumentReference{Office: "CBEX1", Year: "2026", Serial: "C", Number: 28242},
		},
	}
	require.NoError(t, svc.ProcessIntegrationResult(context.Background(), req))

	payload, ok := recorder.completed["create_cdn:abc"]
	require.True(t, ok, "the parked task was not completed; the trader would wait forever")
	assert.Equal(t, "step-abc", recorder.steps["create_cdn:abc"], "the step it was parked on must be the one completed")
	assert.Equal(t, "submit", payload["__command"])
	assert.Equal(t, true, payload["integrated"])
	// The workflow is handed the registered reference, which is what the trader
	// sees and what the acknowledgment is later correlated by.
	assert.Equal(t, "CBEX1/2026/C/28242", payload["cdn_number"])
	assert.NotContains(t, recorder.completed, "create_cdn:other", "another branch's wait must not be resumed")
}

// A failed integration releases the same wait, but carries the reasons instead
// of a reference so the trader is returned to the form knowing why.
func TestProcessIntegrationResult_ResumesWithFailureReasons(t *testing.T) {
	recorder := &resumeRecorder{}
	repo := &mockRepository{byEdgeID: map[string]*DispatchNote{"edge-2": dispatchedNote("edge-2")}}
	svc := NewCDNWebhookService(repo, parkedIntegrationWait(), recorder, nil)

	req := CDNIntegrationResultRequest{
		Event: "CDN_INTEGRATED",
		Payload: integrationResultPayload{
			EdgeID:     "edge-2",
			Integrated: false,
			Errors:     json.RawMessage(`{"0":[{"code":331,"description":"Missing office Code"}]}`),
		},
	}
	require.NoError(t, svc.ProcessIntegrationResult(context.Background(), req))

	payload := recorder.completed["create_cdn:abc"]
	require.NotNil(t, payload)
	assert.Equal(t, false, payload["integrated"])
	assert.NotContains(t, payload, "cdn_number")
	assert.Contains(t, payload["error"], "Missing office Code")
}

func integratedResult(edgeID string) CDNIntegrationResultRequest {
	return CDNIntegrationResultRequest{
		Event: "CDN_INTEGRATED",
		Payload: integrationResultPayload{
			EdgeID:     edgeID,
			Integrated: true,
			CDNRef:     DocumentReference{Office: "CBEX1", Year: "2026", Serial: "C", Number: 28242},
		},
	}
}

// A first delivery whose task has not parked yet is an error, so SLC Edge
// retries it (§2) rather than being told it was handled.
func TestProcessIntegrationResult_TaskNotParkedYetIsRetried(t *testing.T) {
	recorder := &resumeRecorder{}
	repo := &mockRepository{byEdgeID: map[string]*DispatchNote{"edge-1": dispatchedNote("edge-1")}}
	svc := NewCDNWebhookService(repo, fakeTasks{}, recorder, nil)

	err := svc.ProcessIntegrationResult(context.Background(), integratedResult("edge-1"))

	require.ErrorIs(t, err, ErrTaskNotParked)
	assert.Empty(t, recorder.completed)
	assert.Nil(t, repo.updated, "the result must not be recorded, or the retry is a redelivery")
}

// The task is released before the result is recorded, so a retry that finds
// the task parked completes it, and only then is the result recorded.
func TestProcessIntegrationResult_RetryAfterTheTaskParksCompletesIt(t *testing.T) {
	recorder := &resumeRecorder{}
	repo := &mockRepository{byEdgeID: map[string]*DispatchNote{"edge-1": dispatchedNote("edge-1")}}

	early := NewCDNWebhookService(repo, fakeTasks{}, recorder, nil)
	require.ErrorIs(t, early.ProcessIntegrationResult(context.Background(), integratedResult("edge-1")), ErrTaskNotParked)
	assert.Equal(t, DispatchNoteStatusSubmitted, repo.byEdgeID["edge-1"].Status)

	retry := NewCDNWebhookService(repo, parkedIntegrationWait(), recorder, nil)
	require.NoError(t, retry.ProcessIntegrationResult(context.Background(), integratedResult("edge-1")))

	assert.Contains(t, recorder.completed, "create_cdn:abc")
	require.NotNil(t, repo.updated)
	assert.Equal(t, DispatchNoteStatusIntegrated, repo.updated.Status)
}

// failingCompleter is a task manager whose completion fails.
type failingCompleter struct{}

func (failingCompleter) CompleteTaskStep(context.Context, string, string, map[string]any) error {
	return errCompletion
}

var errCompletion = errors.New("complete task step: workflow unavailable")

// A completion that fails leaves the note as it was, so the retry resumes it.
func TestProcessIntegrationResult_FailedCompletionLeavesTheNote(t *testing.T) {
	for name, req := range map[string]CDNIntegrationResultRequest{
		"integrated": integratedResult("edge-1"),
		"failed": {Payload: integrationResultPayload{
			EdgeID: "edge-1", Errors: json.RawMessage(`{"code":"ERR_VAL_01"}`),
		}},
	} {
		t.Run(name, func(t *testing.T) {
			repo := &mockRepository{byEdgeID: map[string]*DispatchNote{"edge-1": dispatchedNote("edge-1")}}
			svc := NewCDNWebhookService(repo, parkedIntegrationWait(), failingCompleter{}, nil)

			err := svc.ProcessIntegrationResult(context.Background(), req)

			require.ErrorIs(t, err, errCompletion)
			assert.Nil(t, repo.updated)
			assert.Equal(t, DispatchNoteStatusSubmitted, repo.byEdgeID["edge-1"].Status)
		})
	}
}

// integratedNote is a note whose integration result was recorded, with the
// acknowledgment wait of case-1's wf-branch-0 to come.
func integratedNote() *DispatchNote {
	note := dispatchedNote("edge-1")
	note.Status = DispatchNoteStatusIntegrated
	note.CDNOffice, note.CDNYear, note.CDNSerial, note.CDNNumber = "CBEX1", "2026", "C", 28242
	return note
}

func acknowledgment() CDNAcknowledgmentRequest {
	return CDNAcknowledgmentRequest{
		Event:   "CDN_ACKNOWLEDGED",
		Payload: acknowledgmentPayload{CDNRef: DocumentReference{Office: "CBEX1", Year: "2026", Serial: "C", Number: 28242}},
	}
}

// An acknowledgment whose wait has not parked yet is retried, and the note
// stays INTEGRATED so the retry can still resume the wait.
func TestProcessAcknowledgment_RetryAfterTheTaskParksCompletesIt(t *testing.T) {
	note := integratedNote()
	recorder := &resumeRecorder{}
	repo := &mockRepository{byCDNRef: map[string]*DispatchNote{"2026-CBEX1-C": note}}

	early := NewCDNWebhookService(repo, fakeTasks{}, recorder, nil)
	require.ErrorIs(t, early.ProcessAcknowledgment(context.Background(), acknowledgment()), ErrTaskNotParked)
	assert.Nil(t, repo.updated)
	assert.Equal(t, DispatchNoteStatusIntegrated, note.Status)

	parked := fakeTasks{"case-1": {{TaskID: "create_cdn:abc", ActiveStepID: "step-ack", ParentWorkflowID: "wf-branch-0",
		CaseID: "case-1", ActiveTaskTemplateID: acknowledgmentWaitTemplateID, State: "QUEUED_EXTERNALLY"}}}
	retry := NewCDNWebhookService(repo, parked, recorder, nil)
	require.NoError(t, retry.ProcessAcknowledgment(context.Background(), acknowledgment()))

	assert.Equal(t, "step-ack", recorder.steps["create_cdn:abc"])
	require.NotNil(t, repo.updated)
	assert.Equal(t, DispatchNoteStatusAcknowledged, repo.updated.Status)
}

// A redelivery that finds nothing parked is acknowledged: the first delivery
// resumed the task.
func TestProcessIntegrationResult_RedeliveryWithNothingParkedIsAcknowledged(t *testing.T) {
	note := dispatchedNote("edge-1")
	note.Status = DispatchNoteStatusIntegrated
	note.CDNOffice, note.CDNYear, note.CDNSerial, note.CDNNumber = "CBEX1", "2026", "C", 28242
	repo := &mockRepository{byEdgeID: map[string]*DispatchNote{"edge-1": note}}
	svc := NewCDNWebhookService(repo, fakeTasks{}, &resumeRecorder{}, nil)

	require.NoError(t, svc.ProcessIntegrationResult(context.Background(), integratedResult("edge-1")))
}

// Tasks that could not be read are an error on a redelivery too: whether the
// task is still parked is not known.
func TestProcessIntegrationResult_TaskStoreFailureIsAnError(t *testing.T) {
	for name, note := range map[string]*DispatchNote{
		"first delivery": dispatchedNote("edge-1"),
		"redelivery": func() *DispatchNote {
			n := dispatchedNote("edge-1")
			n.Status = DispatchNoteStatusIntegrated
			return n
		}(),
	} {
		t.Run(name, func(t *testing.T) {
			recorder := &resumeRecorder{}
			repo := &mockRepository{byEdgeID: map[string]*DispatchNote{"edge-1": note}}
			svc := NewCDNWebhookService(repo, failingTasks{}, recorder, nil)

			err := svc.ProcessIntegrationResult(context.Background(), integratedResult("edge-1"))

			require.ErrorIs(t, err, errTaskStore)
			assert.NotErrorIs(t, err, ErrTaskNotParked)
			assert.Empty(t, recorder.completed)
			assert.Nil(t, repo.updated)
		})
	}
}
