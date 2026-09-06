package cusdec

import (
	"context"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

type mockCusdecRepository struct {
	declsByEdgeID map[string]*CusdecDeclaration
	createCalled  bool
	updateCalled  bool
	createdDecl   *CusdecDeclaration
	updatedDecl   *CusdecDeclaration
}

func (m *mockCusdecRepository) GetByEdgeID(ctx context.Context, edgeID string) (*CusdecDeclaration, error) {
	return m.declsByEdgeID[edgeID], nil
}

func (m *mockCusdecRepository) GetByCusdecRef(ctx context.Context, ref DocumentReference) (*CusdecDeclaration, error) {
	for _, d := range m.declsByEdgeID {
		if d.CusdecOffice == ref.Office && d.CusdecYear == ref.Year && d.CusdecSerial == ref.Serial && d.CusdecNumber == ref.Number {
			return d, nil
		}
	}
	return nil, nil
}

func (m *mockCusdecRepository) Create(ctx context.Context, decl *CusdecDeclaration) error {
	m.createCalled = true
	m.createdDecl = decl
	return nil
}

func (m *mockCusdecRepository) Update(ctx context.Context, decl *CusdecDeclaration) error {
	m.updateCalled = true
	m.updatedDecl = decl
	return nil
}

type mockTaskCompleter struct {
	mock.Mock
}

func (m *mockTaskCompleter) CompleteTaskStep(ctx context.Context, taskID string, payload map[string]any) error {
	args := m.Called(ctx, taskID, payload)
	return args.Error(0)
}

func setupTestDB(t *testing.T) (*gorm.DB, sqlmock.Sqlmock) {
	mockDB, sqlMock, err := sqlmock.New()
	require.NoError(t, err)

	db, err := gorm.Open(postgres.New(postgres.Config{
		Conn: mockDB,
	}), &gorm.Config{})
	require.NoError(t, err)

	return db, sqlMock
}

func TestProcessCusdecIntegrationResult_Success(t *testing.T) {
	ctx := context.Background()
	db, sqlMock := setupTestDB(t)

	repo := &mockCusdecRepository{
		declsByEdgeID: make(map[string]*CusdecDeclaration),
	}
	completer := &mockTaskCompleter{}
	service := NewWebhookService(repo, db, completer)

	req := CusdecIntegrationResultRequest{
		EdgeID:     "edge-123",
		Integrated: true,
		Event:      "INTEGRATION_RESULT",
		ProcessAt:  time.Now(),
		Payload: cusdecResultPayload{
			CusdecRef: DocumentReference{
				Year:   "2026",
				Office: "COL",
				Serial: "C",
				Number: 9876,
			},
			// §6.2 returns the assessed duty alongside the reference; its total
			// is what the trader is asked to settle on the payment step.
			Taxes: []TaxEntry{
				{Code: "tax1", Rate: 1, Amount: 222},
				{Code: "tax2", Rate: 1, Amount: 1022},
			},
		},
	}

	sqlMock.ExpectQuery(`(?i)SELECT.*FROM "task_records_v2"`).
		WithArgs("edge-123", "edge-123", 1).
		WillReturnRows(sqlmock.NewRows([]string{"parent_workflow_id"}).AddRow("parent-wf-123"))

	sqlMock.ExpectQuery(`(?i)SELECT.*FROM "task_records_v2"`).
		WithArgs("parent-wf-123", "customs-cusdec--external-review", "QUEUED_EXTERNALLY", 1).
		WillReturnRows(sqlmock.NewRows([]string{"task_id"}).AddRow("task-abc"))

	expectedPayload := map[string]any{
		"__command":      "submit",
		"review_outcome": "approve",
		"cusdec_number":  "COL/2026/C/9876",
		"amount_to_pay":  float64(1244),
	}
	completer.On("CompleteTaskStep", mock.Anything, "task-abc", expectedPayload).Return(nil)

	err := service.ProcessIntegrationResult(ctx, req)
	require.NoError(t, err)

	assert.True(t, repo.createCalled)
	assert.Equal(t, CusdecStatusIntegrated, repo.createdDecl.Status)
	assert.Equal(t, "COL", repo.createdDecl.CusdecOffice)
	assert.Equal(t, 9876, repo.createdDecl.CusdecNumber)

	completer.AssertExpectations(t)
	require.NoError(t, sqlMock.ExpectationsWereMet())
}

func TestProcessEvent_PaymentSuccess(t *testing.T) {
	ctx := context.Background()
	db, sqlMock := setupTestDB(t)

	decl := &CusdecDeclaration{
		EdgeID:       "edge-123",
		CusdecOffice: "COL",
		CusdecYear:   "2026",
		CusdecSerial: "C",
		CusdecNumber: 9876,
		Status:       CusdecStatusIntegrated,
	}
	repo := &mockCusdecRepository{
		declsByEdgeID: map[string]*CusdecDeclaration{
			"edge-123": decl,
		},
	}
	completer := &mockTaskCompleter{}
	service := NewWebhookService(repo, db, completer)

	req := CusdecEventRequest{
		Event:     "PAYMENT_CONFIRMED",
		ProcessAt: time.Now(),
		Payload: cusdecEventPayload{
			CusdecRef: DocumentReference{
				Year:   "2026",
				Office: "COL",
				Serial: "C",
				Number: 9876,
			},
			AmountPaid:    2035.00,
			Currency:      "LKR",
			BankReference: "84004328",
		},
	}

	sqlMock.ExpectQuery(`(?i)SELECT.*FROM "task_records_v2"`).
		WithArgs("edge-123", "edge-123", 1).
		WillReturnRows(sqlmock.NewRows([]string{"parent_workflow_id"}).AddRow("parent-wf-123"))

	sqlMock.ExpectQuery(`(?i)SELECT.*FROM "task_records_v2"`).
		WithArgs("parent-wf-123", "customs-wait-payment", "QUEUED_EXTERNALLY", 1).
		WillReturnRows(sqlmock.NewRows([]string{"task_id"}).AddRow("task-payment-123"))

	expectedPayload := map[string]any{
		"__command":      "submit",
		"payment_status": "PAID",
		// §6.5.1 fields reach the trader's receipt, so they must survive the
		// hop from callback to task rather than being parsed and dropped.
		"amount_paid":    2035.00,
		"currency":       "LKR",
		"bank_reference": "84004328",
	}
	completer.On("CompleteTaskStep", mock.Anything, "task-payment-123", expectedPayload).Return(nil)

	err := service.ProcessEvent(ctx, req)
	require.NoError(t, err)

	assert.True(t, repo.updateCalled)
	assert.Equal(t, CusdecStatusPaid, repo.updatedDecl.Status)
	completer.AssertExpectations(t)
	require.NoError(t, sqlMock.ExpectationsWereMet())
}

func TestProcessEvent_DuplicateEventIsAcknowledgedWithoutAction(t *testing.T) {
	ctx := context.Background()
	db, sqlMock := setupTestDB(t)

	decl := &CusdecDeclaration{
		EdgeID:       "edge-123",
		CusdecOffice: "COL",
		CusdecYear:   "2026",
		CusdecSerial: "C",
		CusdecNumber: 9876,
		Status:       CusdecStatusPaid,
	}
	repo := &mockCusdecRepository{
		declsByEdgeID: map[string]*CusdecDeclaration{
			"edge-123": decl,
		},
	}
	completer := &mockTaskCompleter{}
	service := NewWebhookService(repo, db, completer)

	req := CusdecEventRequest{
		Event:     "PAYMENT_CONFIRMED",
		ProcessAt: time.Now(),
		Payload: cusdecEventPayload{
			CusdecRef: DocumentReference{
				Year:   "2026",
				Office: "COL",
				Serial: "C",
				Number: 9876,
			},
		},
	}

	err := service.ProcessEvent(ctx, req)

	// The declaration is already PAID, so this PAYMENT_CONFIRMED has been
	// applied before: acknowledged without touching the declaration or its
	// workflow, and without so much as a lookup.
	require.ErrorIs(t, err, ErrDuplicateEvent)

	assert.False(t, repo.updateCalled)
	completer.AssertExpectations(t)
	require.NoError(t, sqlMock.ExpectationsWereMet())
}

func TestProcessEvent_WarrantingSuccess(t *testing.T) {
	ctx := context.Background()
	db, sqlMock := setupTestDB(t)

	decl := &CusdecDeclaration{
		EdgeID:       "edge-123",
		CusdecOffice: "COL",
		CusdecYear:   "2026",
		CusdecSerial: "C",
		CusdecNumber: 9876,
		Status:       CusdecStatusPaid,
	}
	repo := &mockCusdecRepository{
		declsByEdgeID: map[string]*CusdecDeclaration{
			"edge-123": decl,
		},
	}
	completer := &mockTaskCompleter{}
	service := NewWebhookService(repo, db, completer)

	req := CusdecEventRequest{
		Event:     "WARRANTING_COMPLETED",
		ProcessAt: time.Now(),
		Payload: cusdecEventPayload{
			CusdecRef: DocumentReference{
				Year:   "2026",
				Office: "COL",
				Serial: "C",
				Number: 9876,
			},
		},
	}

	sqlMock.ExpectQuery(`(?i)SELECT.*FROM "task_records_v2"`).
		WithArgs("edge-123", "edge-123", 1).
		WillReturnRows(sqlmock.NewRows([]string{"parent_workflow_id"}).AddRow("parent-wf-123"))

	sqlMock.ExpectQuery(`(?i)SELECT.*FROM "task_records_v2"`).
		WithArgs("parent-wf-123", "customs-wait-warranting", "QUEUED_EXTERNALLY", 1).
		WillReturnRows(sqlmock.NewRows([]string{"task_id"}).AddRow("task-warranting-123"))

	expectedPayload := map[string]any{
		"__command":         "submit",
		"warranting_status": "WARRANTED",
		"release_order_no":  "",
	}
	completer.On("CompleteTaskStep", mock.Anything, "task-warranting-123", expectedPayload).Return(nil)

	err := service.ProcessEvent(ctx, req)
	require.NoError(t, err)

	assert.True(t, repo.updateCalled)
	assert.Equal(t, CusdecStatusWarranted, repo.updatedDecl.Status)
	completer.AssertExpectations(t)
	require.NoError(t, sqlMock.ExpectationsWereMet())
}

func TestProcessEvent_ReleaseSuccess(t *testing.T) {
	ctx := context.Background()
	db, sqlMock := setupTestDB(t)

	decl := &CusdecDeclaration{
		EdgeID:       "edge-123",
		CusdecOffice: "COL",
		CusdecYear:   "2026",
		CusdecSerial: "C",
		CusdecNumber: 9876,
		Status:       CusdecStatusWarranted,
	}
	repo := &mockCusdecRepository{
		declsByEdgeID: map[string]*CusdecDeclaration{
			"edge-123": decl,
		},
	}
	completer := &mockTaskCompleter{}
	service := NewWebhookService(repo, db, completer)

	req := CusdecEventRequest{
		Event:     "EXPORT_RELEASED",
		ProcessAt: time.Now(),
		Payload: cusdecEventPayload{
			CusdecRef: DocumentReference{
				Year:   "2026",
				Office: "COL",
				Serial: "C",
				Number: 9876,
			},
			VesselName:    "EVER GIVEN",
			VoyageNo:      "023W 08/01/2025",
			PortOfLoading: "LKCMB",
		},
	}

	sqlMock.ExpectQuery(`(?i)SELECT.*FROM "task_records_v2"`).
		WithArgs("edge-123", "edge-123", 1).
		WillReturnRows(sqlmock.NewRows([]string{"parent_workflow_id"}).AddRow("parent-wf-123"))

	sqlMock.ExpectQuery(`(?i)SELECT.*FROM "task_records_v2"`).
		WithArgs("parent-wf-123", "customs-wait-export-release", "QUEUED_EXTERNALLY", 1).
		WillReturnRows(sqlmock.NewRows([]string{"task_id"}).AddRow("task-release-123"))

	expectedPayload := map[string]any{
		"__command":       "submit",
		"release_status":  "RELEASED",
		"vessel_name":     "EVER GIVEN",
		"voyage_no":       "023W 08/01/2025",
		"port_of_loading": "LKCMB",
	}
	completer.On("CompleteTaskStep", mock.Anything, "task-release-123", expectedPayload).Return(nil)

	err := service.ProcessEvent(ctx, req)
	require.NoError(t, err)

	assert.True(t, repo.updateCalled)
	assert.Equal(t, CusdecStatusReleased, repo.updatedDecl.Status)
	completer.AssertExpectations(t)
	require.NoError(t, sqlMock.ExpectationsWereMet())
}

// The edgeId threads one round-trip; cusdecRef is the declaration itself. Two
// edgeIds resolving to one reference means ASYCUDA answered twice for the same
// registered declaration — recording it again would give one CusDec two rows
// and complete its review a second time.
func TestProcessCusdecIntegrationResult_ReferenceAlreadyHeldIsAcknowledged(t *testing.T) {
	ctx := context.Background()
	db, _ := setupTestDB(t)

	ref := DocumentReference{Year: "2026", Office: "COL", Serial: "C", Number: 9876}
	repo := &mockCusdecRepository{declsByEdgeID: map[string]*CusdecDeclaration{
		// Registered under an earlier correlation id, and already answered.
		"edge-first": {
			ID: "decl-1", EdgeID: "edge-first", Status: CusdecStatusIntegrated,
			CusdecYear: ref.Year, CusdecOffice: ref.Office, CusdecSerial: ref.Serial, CusdecNumber: ref.Number,
		},
	}}
	completer := &mockTaskCompleter{}
	service := NewWebhookService(repo, db, completer)

	err := service.ProcessIntegrationResult(ctx, CusdecIntegrationResultRequest{
		EdgeID:     "edge-second",
		Integrated: true,
		Event:      "INTEGRATION_RESULT",
		ProcessAt:  time.Now(),
		Payload:    cusdecResultPayload{CusdecRef: ref},
	})

	require.ErrorIs(t, err, ErrDuplicateRegisteredReference)
	assert.False(t, repo.createCalled, "a second row for one declaration is what this prevents")
	assert.False(t, repo.updateCalled)
	completer.AssertNotCalled(t, "CompleteTaskStep", mock.Anything, mock.Anything, mock.Anything)
}

// The same edgeId answering again is the retry schedule, judged where it always
// was — the reference check must not shadow it with a different error.
func TestProcessCusdecIntegrationResult_SameEdgeIDKeepsItsOwnDuplicateError(t *testing.T) {
	ctx := context.Background()
	db, _ := setupTestDB(t)

	ref := DocumentReference{Year: "2026", Office: "COL", Serial: "C", Number: 9876}
	repo := &mockCusdecRepository{declsByEdgeID: map[string]*CusdecDeclaration{
		"edge-123": {
			ID: "decl-1", EdgeID: "edge-123", Status: CusdecStatusIntegrated,
			CusdecYear: ref.Year, CusdecOffice: ref.Office, CusdecSerial: ref.Serial, CusdecNumber: ref.Number,
		},
	}}
	service := NewWebhookService(repo, db, &mockTaskCompleter{})

	err := service.ProcessIntegrationResult(ctx, CusdecIntegrationResultRequest{
		EdgeID: "edge-123", Integrated: true, Event: "INTEGRATION_RESULT",
		ProcessAt: time.Now(), Payload: cusdecResultPayload{CusdecRef: ref},
	})

	require.ErrorIs(t, err, ErrDuplicateIntegrationResult)
}

// A rejection carries no reference (§6.2), so there is nothing to compare and
// the declaration keeps its edgeId as its only identity.
func TestProcessCusdecIntegrationResult_FailureIsNotComparedByReference(t *testing.T) {
	ctx := context.Background()
	db, _ := setupTestDB(t)

	repo := &mockCusdecRepository{declsByEdgeID: map[string]*CusdecDeclaration{
		"edge-first": {
			ID: "decl-1", EdgeID: "edge-first", Status: CusdecStatusIntegrated,
			CusdecYear: "2026", CusdecOffice: "COL", CusdecSerial: "C", CusdecNumber: 9876,
		},
	}}
	service := NewWebhookService(repo, db, &mockTaskCompleter{})

	err := service.ProcessIntegrationResult(ctx, CusdecIntegrationResultRequest{
		EdgeID: "edge-second", Integrated: false, Event: "INTEGRATION_RESULT",
		ProcessAt: time.Now(), Payload: cusdecResultPayload{},
	})

	assert.NotErrorIs(t, err, ErrDuplicateRegisteredReference)
}

// A reference held by a declaration that failed belongs to a submission ASYCUDA
// rejected. The trader corrects it and resubmits, and that new round-trip has a
// new edgeId against the same reference — it must be recorded, not refused.
func TestProcessCusdecIntegrationResult_ReferenceHeldByAFailedDeclarationIsNotADuplicate(t *testing.T) {
	ctx := context.Background()
	db, sqlMock := setupTestDB(t)

	ref := DocumentReference{Year: "2026", Office: "COL", Serial: "C", Number: 9876}
	repo := &mockCusdecRepository{declsByEdgeID: map[string]*CusdecDeclaration{
		"edge-first": {
			ID: "decl-1", EdgeID: "edge-first", Status: CusdecStatusFailed,
			CusdecYear: ref.Year, CusdecOffice: ref.Office, CusdecSerial: ref.Serial, CusdecNumber: ref.Number,
		},
	}}
	sqlMock.ExpectQuery(`(?i)SELECT.*FROM "task_records_v2"`).
		WithArgs("edge-second", "edge-second", 1).
		WillReturnRows(sqlmock.NewRows([]string{"parent_workflow_id"}).AddRow("parent-wf-1"))
	sqlMock.ExpectQuery(`(?i)SELECT.*FROM "task_records_v2"`).
		WithArgs("parent-wf-1", "customs-cusdec--external-review", "QUEUED_EXTERNALLY", 1).
		WillReturnRows(sqlmock.NewRows([]string{"task_id"}).AddRow("task-abc"))

	completer := &mockTaskCompleter{}
	completer.On("CompleteTaskStep", mock.Anything, mock.Anything, mock.Anything).Return(nil)
	service := NewWebhookService(repo, db, completer)

	err := service.ProcessIntegrationResult(ctx, CusdecIntegrationResultRequest{
		EdgeID: "edge-second", Integrated: true, Event: "INTEGRATION_RESULT",
		ProcessAt: time.Now(), Payload: cusdecResultPayload{CusdecRef: ref},
	})

	assert.NotErrorIs(t, err, ErrDuplicateRegisteredReference,
		"the corrected resubmission is a new round-trip and belongs on the record")
}
