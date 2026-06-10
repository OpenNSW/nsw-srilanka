package consignment

import (
	"context"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/OpenNSW/core/taskflow/store"
	workflow "github.com/OpenNSW/core/workflow"
	"github.com/OpenNSW/nsw-srilanka/internal/profile/company"
)

// MockCompanyService implements company.Service for testing.
type MockCompanyService struct {
	mock.Mock
}

func (m *MockCompanyService) GetCompanyByID(ctx context.Context, id string) (*company.Record, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*company.Record), args.Error(1)
}

func (m *MockCompanyService) GetCompanyByOUHandle(ctx context.Context, ouHandle string) (*company.Record, error) {
	args := m.Called(ctx, ouHandle)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*company.Record), args.Error(1)
}

func (m *MockCompanyService) ListCompanies(ctx context.Context, filter company.ListFilter) (*company.ListResult, error) {
	args := m.Called(ctx, filter)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*company.ListResult), args.Error(1)
}

func (m *MockCompanyService) UpdateCompany(ctx context.Context, id string, data map[string]any) error {
	return m.Called(ctx, id, data).Error(0)
}

func (m *MockCompanyService) Health(ctx context.Context) error {
	return m.Called(ctx).Error(0)
}

func TestConsignmentService_RegisterWorkflowManager(t *testing.T) {
	db, _ := setupTestDB(t)
	svc := NewService(db, nil, nil, nil, nil, nil)
	mockWM := new(MockWM)

	// Test registration
	err := svc.RegisterWorkflowManager(mockWM)
	assert.NoError(t, err)

	// Test already registered
	err = svc.RegisterWorkflowManager(mockWM)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "already registered")

	// Test nil manager
	svc2 := NewService(db, nil, nil, nil, nil, nil)
	err = svc2.RegisterWorkflowManager(nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "cannot be nil")
}

func TestConsignmentService_CompletionHandler(t *testing.T) {
	db, sqlMock := setupTestDB(t)
	svc := NewService(db, nil, nil, nil, nil, nil)
	consignmentID := uuid.NewString()

	sqlMock.ExpectQuery(`SELECT \* FROM "consignments" WHERE id = \$1`).
		WithArgs(consignmentID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "state"}).AddRow(consignmentID, "IN_PROGRESS"))
	sqlMock.ExpectBegin()
	sqlMock.ExpectExec(`UPDATE "consignments"`).
		WillReturnResult(sqlmock.NewResult(1, 1))
	sqlMock.ExpectCommit()

	err := svc.CompletionHandler(consignmentID, nil)
	assert.NoError(t, err)
	assert.NoError(t, sqlMock.ExpectationsWereMet())
}

func TestConsignmentService_GetConsignmentByID(t *testing.T) {
	db, sqlMock := setupTestDB(t)
	mockWM := new(MockWM)
	mockTaskStore := new(MockTaskStore)
	svc := NewService(db, nil, nil, nil, nil, mockTaskStore)
	require.NoError(t, svc.RegisterWorkflowManager(mockWM))

	ctx := context.Background()
	consignmentID := uuid.NewString()

	sqlMock.ExpectQuery(`SELECT \* FROM "consignments" WHERE id = \$1 ORDER BY "consignments"."id" LIMIT \$2`).
		WithArgs(consignmentID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "flow", "trader_id", "state", "created_at", "updated_at"}).
			AddRow(consignmentID, "IMPORT", "trader1", "IN_PROGRESS", time.Now(), time.Now()))

	mockWM.On("GetStatus", ctx, consignmentID).Return((*workflow.WorkflowInstance)(nil), nil)
	mockTaskStore.On("GetAllTasks", mock.Anything, consignmentID).Return(([]store.TaskRecord)(nil))

	result, err := svc.GetConsignmentByID(ctx, consignmentID)
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, consignmentID, result.ID)
	mockWM.AssertExpectations(t)
	mockTaskStore.AssertExpectations(t)
	assert.NoError(t, sqlMock.ExpectationsWereMet())
}

func TestConsignmentService_ListConsignments_TraderCompany_Empty(t *testing.T) {
	db, sqlMock := setupTestDB(t)
	svc := NewService(db, nil, nil, nil, nil, nil)
	ctx := context.Background()
	companyID := "company-1"

	sqlMock.ExpectQuery(`SELECT \* FROM "consignments" WHERE trader_company_id = \$1`).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))

	result, err := svc.ListConsignments(ctx, Filter{TraderCompanyID: &companyID})
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, int64(0), result.Total)
	assert.Empty(t, result.Items)
	assert.NoError(t, sqlMock.ExpectationsWereMet())
}
