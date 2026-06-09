package consignment

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/OpenNSW/core/artifact"
	"github.com/OpenNSW/core/taskflow/store"
	workflow "github.com/OpenNSW/core/workflow"
	"github.com/OpenNSW/nsw/backend/internal/profile/cha"
	"github.com/OpenNSW/nsw/backend/internal/profile/company"
	"github.com/OpenNSW/nsw/backend/internal/profile/user"
)

// MockCHAService implements cha.Service for testing.
type MockCHAService struct {
	mock.Mock
}

func (m *MockCHAService) GetByID(ctx context.Context, id string) (*cha.Record, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*cha.Record), args.Error(1)
}

func (m *MockCHAService) GetByEmail(ctx context.Context, email string) (*cha.Record, error) {
	args := m.Called(ctx, email)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*cha.Record), args.Error(1)
}

func (m *MockCHAService) List(ctx context.Context) ([]cha.Record, error) {
	args := m.Called(ctx)
	return args.Get(0).([]cha.Record), args.Error(1)
}

func (m *MockCHAService) Health() error {
	return m.Called().Error(0)
}

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

// MockUserService implements user.Service for testing.
type MockUserService struct {
	mock.Mock
}

func (m *MockUserService) GetUser(id string) (*user.Record, error) {
	args := m.Called(id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*user.Record), args.Error(1)
}

func (m *MockUserService) GetOrCreateUser(idpUserID, email, phone, ouID, ouHandle string) (*string, error) {
	args := m.Called(idpUserID, email, phone, ouID, ouHandle)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*string), args.Error(1)
}

func (m *MockUserService) UpdateUserData(id string, data []byte) error {
	return m.Called(id, data).Error(0)
}

func (m *MockUserService) Health() error {
	return m.Called().Error(0)
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

func TestConsignmentService_InitializeConsignmentByID_NoTemplateID(t *testing.T) {
	svc := NewService(nil, nil, nil, nil, nil, nil)
	_, err := svc.InitializeConsignmentByID(context.Background(), "id", "", "")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "template ID is required")
}

func TestConsignmentService_InitializeConsignmentByID_NotFound(t *testing.T) {
	db, sqlMock := setupTestDB(t)
	svc := NewService(db, nil, nil, nil, nil, nil)
	id := uuid.NewString()
	sqlMock.ExpectQuery(`SELECT \* FROM "consignments" WHERE id = \$1`).
		WithArgs(id, 1).
		WillReturnError(gorm.ErrRecordNotFound)

	_, err := svc.InitializeConsignmentByID(context.Background(), id, "tmpl-1", "")
	assert.Error(t, err)
	assert.True(t, errors.Is(err, gorm.ErrRecordNotFound))
}

func TestConsignmentService_InitializeConsignmentByID_WrongState(t *testing.T) {
	db, sqlMock := setupTestDB(t)
	svc := NewService(db, nil, nil, nil, nil, nil)
	id := uuid.NewString()
	sqlMock.ExpectQuery(`SELECT \* FROM "consignments" WHERE id = \$1`).
		WithArgs(id, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "state"}).AddRow(id, "IN_PROGRESS"))

	_, err := svc.InitializeConsignmentByID(context.Background(), id, "tmpl-1", "")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "must be in INITIALIZED")
}

func TestConsignmentService_InitializeConsignmentByID_CHACompanyMismatch(t *testing.T) {
	db, sqlMock := setupTestDB(t)
	mockCHA := new(MockCHAService)
	mockCompany := new(MockCompanyService)
	svc := NewService(db, nil, mockCHA, mockCompany, nil, nil)

	id := uuid.NewString()
	chaID := "cha1"

	sqlMock.ExpectQuery(`SELECT \* FROM "consignments" WHERE id = \$1`).
		WithArgs(id, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "state", "flow", "cha_company_id"}).AddRow(id, "INITIALIZED", "IMPORT", "company-A"))

	mockCHA.On("GetByID", mock.Anything, chaID).Return(&cha.Record{ID: chaID, CompanyID: "company-B"}, nil)

	_, err := svc.InitializeConsignmentByID(context.Background(), id, "tmpl-1", chaID)
	assert.ErrorIs(t, err, ErrCHACompanyMismatch)
}

func TestConsignmentService_InitializeConsignmentByID_Success(t *testing.T) {
	db, sqlMock := setupTestDB(t)
	mockCHA := new(MockCHAService)
	mockCompany := new(MockCompanyService)
	mockTaskStore := new(MockTaskStore)
	mockWM := new(MockWM)

	// Set up artifact registry
	reg := artifact.NewRegistry()
	loader := &mockLoader{content: make(map[string][]byte)}
	reg.RegisterLoader("test", loader)
	// Register a mock workflow definition
	wtID := "tmpl-1"
	loader.content["workflows/tmpl-1"] = []byte(`{"id":"tmpl-1","name":"Test Workflow"}`)
	reg.RegisterArtifact(wtID, "workflow", "", "test", "workflows/tmpl-1")

	svc := NewService(db, reg, mockCHA, mockCompany, nil, mockTaskStore)
	require.NoError(t, svc.RegisterWorkflowManager(mockWM))

	id := uuid.NewString()
	traderID := "trader1"
	chaID := "cha1"
	chaCompanyID := "company-cha"
	traderCompanyID := "company-trader"

	sqlMock.ExpectQuery(`SELECT \* FROM "consignments" WHERE id = \$1`).
		WithArgs(id, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "state", "flow", "trader_id", "cha_company_id", "trader_company_id"}).AddRow(id, "INITIALIZED", "IMPORT", traderID, chaCompanyID, traderCompanyID))

	mockCHA.On("GetByID", mock.Anything, chaID).Return(&cha.Record{ID: chaID, CompanyID: chaCompanyID}, nil)
	mockCompany.On("GetCompanyByID", mock.Anything, traderCompanyID).Return(&company.Record{ID: traderCompanyID, OUHandle: "trader-ou", Data: []byte(`{"br_no":"BR-1"}`)}, nil)

	sqlMock.ExpectBegin()
	sqlMock.ExpectExec(`UPDATE "consignments"`).WillReturnResult(sqlmock.NewResult(1, 1))

	mockWM.On("StartWorkflow", mock.Anything, id, workflow.WorkflowDefinition{ID: wtID, Name: "Test Workflow"}, mock.MatchedBy(func(vars map[string]any) bool {
		tc, ok := vars["traderCompany"].(map[string]any)
		return ok && tc["id"] == traderCompanyID
	})).Return(nil)

	sqlMock.ExpectCommit()

	sqlMock.ExpectQuery(`SELECT \* FROM "consignments" WHERE id = \$1`).
		WithArgs(id, id, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "state", "flow", "trader_id", "cha_company_id", "trader_company_id", "cha_id", "created_at", "updated_at"}).
			AddRow(id, "IN_PROGRESS", "IMPORT", traderID, chaCompanyID, traderCompanyID, chaID, time.Now(), time.Now()))

	mockWM.On("GetStatus", mock.Anything, id).Return(&workflow.WorkflowInstance{
		ID: id,
	}, nil)

	mockTaskStore.On("GetAllTasks", mock.Anything, id).Return([]store.TaskRecord{
		{TaskID: "node1", TaskType: "FORM", State: "COMPLETED", ActiveTaskTemplateID: "Task 1", CreatedAt: time.Now(), UpdatedAt: time.Now()},
	})

	result, err := svc.InitializeConsignmentByID(context.Background(), id, wtID, chaID)
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, id, result.ID)
	assert.Len(t, result.WorkflowNodes, 1)
	assert.Equal(t, "Task 1", result.WorkflowNodes[0].WorkflowNodeTemplate.Name)
	mockCHA.AssertExpectations(t)
	mockCompany.AssertExpectations(t)
	mockTaskStore.AssertExpectations(t)
}

func TestConsignmentService_CreateConsignmentShell_Success(t *testing.T) {
	db, sqlMock := setupTestDB(t)
	mockCompany := new(MockCompanyService)
	mockUser := new(MockUserService)
	mockTaskStore := new(MockTaskStore)
	svc := NewService(db, nil, nil, mockCompany, mockUser, mockTaskStore)
	ctx := context.Background()
	chaCompanyID := uuid.NewString()
	traderCompanyID := uuid.NewString()
	consignmentID := uuid.NewString()
	traderID := "trader1"

	mockCompany.On("GetCompanyByID", ctx, chaCompanyID).Return(&company.Record{ID: chaCompanyID, HasCHA: true}, nil)
	mockUser.On("GetUser", traderID).Return(&user.Record{ID: traderID, OUHandle: "trader-ou"}, nil)
	mockCompany.On("GetCompanyByOUHandle", ctx, "trader-ou").Return(&company.Record{ID: traderCompanyID}, nil)

	sqlMock.ExpectBegin()
	sqlMock.ExpectExec(`INSERT INTO "consignments"`).
		WillReturnResult(sqlmock.NewResult(1, 1))
	sqlMock.ExpectCommit()

	sqlMock.ExpectQuery(`SELECT \* FROM "consignments" WHERE id = \$1`).
		WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "flow", "trader_id", "trader_company_id", "cha_company_id", "state"}).
			AddRow(consignmentID, "IMPORT", traderID, traderCompanyID, chaCompanyID, "INITIALIZED"))

	mockTaskStore.On("GetAllTasks", mock.Anything, consignmentID).Return(([]store.TaskRecord)(nil))

	result, err := svc.CreateConsignmentShell(ctx, FlowImport, chaCompanyID, traderID)
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, consignmentID, result.ID)
	mockCompany.AssertExpectations(t)
	mockUser.AssertExpectations(t)
	mockTaskStore.AssertExpectations(t)
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
