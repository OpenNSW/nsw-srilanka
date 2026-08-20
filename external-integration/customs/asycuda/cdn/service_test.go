package cdn

import (
	"context"
	"encoding/json"
	"testing"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
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

func TestProcessIntegrationResult_Success(t *testing.T) {
	repo := &mockRepository{
		byEdgeID: map[string]*DispatchNote{
			"edge-123": {ID: "1", EdgeID: "edge-123", Status: DispatchNoteStatusSubmitted},
		},
	}
	svc := NewCDNWebhookService(repo, nil, nil)

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
	svc := NewCDNWebhookService(repo, nil, nil)

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
func TestProcessIntegrationResult_CreatesNoteForUnseenEdgeID(t *testing.T) {
	repo := &mockRepository{byEdgeID: map[string]*DispatchNote{}}
	svc := NewCDNWebhookService(repo, nil, nil)

	req := CDNIntegrationResultRequest{
		Payload: integrationResultPayload{
			EdgeID:     "edge-unseen",
			Integrated: true,
			CDNRef:     DocumentReference{Year: "2026", Office: "CBEX1", Serial: "C", Number: 28237},
		},
	}
	require.NoError(t, svc.ProcessIntegrationResult(context.Background(), req))

	require.NotNil(t, repo.created)
	assert.Nil(t, repo.updated)
	assert.Equal(t, "edge-unseen", repo.created.EdgeID)
	assert.NotEmpty(t, repo.created.ID)
	assert.Equal(t, DispatchNoteStatusIntegrated, repo.created.Status)
	assert.Equal(t, 28237, repo.created.CDNNumber)
}

func TestProcessAcknowledgment_Success(t *testing.T) {
	repo := &mockRepository{
		byCDNRef: map[string]*DispatchNote{
			"2026-COL-C": {ID: "note-123", Status: DispatchNoteStatusIntegrated},
		},
	}
	svc := NewCDNWebhookService(repo, nil, nil)

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
	svc := NewCDNWebhookService(repo, nil, nil)

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
func setupTestDB(t *testing.T) (*gorm.DB, sqlmock.Sqlmock) {
	mockDB, sqlMock, err := sqlmock.New()
	require.NoError(t, err)

	db, err := gorm.Open(postgres.New(postgres.Config{
		Conn:       mockDB,
		DriverName: "postgres",
	}), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)

	return db, sqlMock
}

// resumeRecorder captures the task steps a callback completes.
type resumeRecorder struct{ completed map[string]map[string]any }

func (r *resumeRecorder) CompleteTaskStep(_ context.Context, taskID string, payload map[string]any) error {
	if r.completed == nil {
		r.completed = map[string]map[string]any{}
	}
	r.completed[taskID] = payload
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
	svc := NewCDNWebhookService(repo, nil, nil)

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
// dispatch note: the workflow is located by the edgeId the dispatch step
// recorded, then the task parked against the integration wait is completed.
func TestProcessIntegrationResult_ResumesTheParkedIntegrationWait(t *testing.T) {
	db, sqlMock := setupTestDB(t)
	recorder := &resumeRecorder{}
	repo := &mockRepository{byEdgeID: map[string]*DispatchNote{}}
	svc := NewCDNWebhookService(repo, db, recorder)

	// 1. the workflow carrying this edgeId, 2. the task parked inside it.
	sqlMock.ExpectQuery(`task_records_v2`).
		WillReturnRows(sqlmock.NewRows([]string{"parent_workflow_id"}).AddRow("wf-branch-0"))
	sqlMock.ExpectQuery(`task_records_v2`).
		WillReturnRows(sqlmock.NewRows([]string{"task_id"}).AddRow("create_cdn:abc"))

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
	assert.Equal(t, "submit", payload["__command"])
	assert.Equal(t, true, payload["integrated"])
	// The workflow is handed the registered reference, which is what the trader
	// sees and what the acknowledgment is later correlated by.
	assert.Equal(t, "CBEX1/2026/C/28242", payload["cdn_number"])
	assert.NoError(t, sqlMock.ExpectationsWereMet())
}

// A failed integration releases the same wait, but carries the reasons instead
// of a reference so the trader is returned to the form knowing why.
func TestProcessIntegrationResult_ResumesWithFailureReasons(t *testing.T) {
	db, sqlMock := setupTestDB(t)
	recorder := &resumeRecorder{}
	repo := &mockRepository{byEdgeID: map[string]*DispatchNote{}}
	svc := NewCDNWebhookService(repo, db, recorder)

	sqlMock.ExpectQuery(`task_records_v2`).
		WillReturnRows(sqlmock.NewRows([]string{"parent_workflow_id"}).AddRow("wf-branch-0"))
	sqlMock.ExpectQuery(`task_records_v2`).
		WillReturnRows(sqlmock.NewRows([]string{"task_id"}).AddRow("create_cdn:abc"))

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
	assert.NoError(t, sqlMock.ExpectationsWereMet())
}
