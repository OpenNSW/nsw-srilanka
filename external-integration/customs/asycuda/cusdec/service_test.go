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
		"amount_to_pay":  0,
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
		Event:     "PAYMENT",
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
		WithArgs("parent-wf-123", "customs-wait-payment", "QUEUED_EXTERNALLY", 1).
		WillReturnRows(sqlmock.NewRows([]string{"task_id"}).AddRow("task-payment-123"))

	expectedPayload := map[string]any{
		"__command":      "submit",
		"payment_status": "PAID",
	}
	completer.On("CompleteTaskStep", mock.Anything, "task-payment-123", expectedPayload).Return(nil)

	err := service.ProcessEvent(ctx, req)
	require.NoError(t, err)

	assert.True(t, repo.updateCalled)
	assert.Equal(t, CusdecStatusPaid, repo.updatedDecl.Status)
	completer.AssertExpectations(t)
	require.NoError(t, sqlMock.ExpectationsWereMet())
}

func TestProcessEvent_DuplicateCallback_WorkflowFinished(t *testing.T) {
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
		Event:     "PAYMENT",
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
		WillReturnError(gorm.ErrRecordNotFound)

	err := service.ProcessEvent(ctx, req)
	require.NoError(t, err)

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
		Event:     "WARRANTING",
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
		Event:     "RELEASE",
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
		WithArgs("parent-wf-123", "customs-wait-release", "QUEUED_EXTERNALLY", 1).
		WillReturnRows(sqlmock.NewRows([]string{"task_id"}).AddRow("task-release-123"))

	expectedPayload := map[string]any{
		"__command":      "submit",
		"release_status": "RELEASED",
	}
	completer.On("CompleteTaskStep", mock.Anything, "task-release-123", expectedPayload).Return(nil)

	err := service.ProcessEvent(ctx, req)
	require.NoError(t, err)

	assert.True(t, repo.updateCalled)
	assert.Equal(t, CusdecStatusReleased, repo.updatedDecl.Status)
	completer.AssertExpectations(t)
	require.NoError(t, sqlMock.ExpectationsWereMet())
}
