package consignment

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/OpenNSW/core/artifact"
	"github.com/OpenNSW/core/taskflow/store"
	workflow "github.com/OpenNSW/core/workflow"
	"github.com/OpenNSW/nsw-srilanka/internal/profile/company"
	"github.com/OpenNSW/nsw-srilanka/internal/profile/user"
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

func (m *MockCompanyService) CreateCompany(ctx context.Context, record *company.Record) error {
	return m.Called(ctx, record).Error(0)
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
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "flow", "trader_id", "state", "created_at", "updated_at"}).
			AddRow(consignmentID, "My Test Consignment", "IMPORT", "trader1", "IN_PROGRESS", time.Now(), time.Now()))

	mockWM.On("GetStatus", ctx, consignmentID).Return((*workflow.WorkflowInstance)(nil), nil)
	mockTaskStore.On("GetAllTasks", mock.Anything, consignmentID).Return(([]store.TaskRecord)(nil))

	result, err := svc.GetConsignmentByID(ctx, consignmentID)
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, consignmentID, result.ID)
	assert.Equal(t, "My Test Consignment", result.Name)
	mockWM.AssertExpectations(t)
	mockTaskStore.AssertExpectations(t)
	assert.NoError(t, sqlMock.ExpectationsWereMet())
}

func TestConsignmentService_ListConsignments_WithQuery(t *testing.T) {
	db, sqlMock := setupTestDB(t)
	svc := NewService(db, nil, nil, nil, nil, nil)
	ctx := context.Background()
	companyID := "company-1"
	query := "abc123"

	sqlMock.ExpectQuery(`(?i)SELECT .* FROM "consignments".*LIKE`).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))

	result, err := svc.ListConsignments(ctx, Filter{TraderCompanyID: &companyID, Query: &query})
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Empty(t, result.Items)
	assert.NoError(t, sqlMock.ExpectationsWereMet())
}

func TestConsignmentService_ListConsignments_EmptyQuery(t *testing.T) {
	db, sqlMock := setupTestDB(t)
	svc := NewService(db, nil, nil, nil, nil, nil)
	ctx := context.Background()
	companyID := "company-1"
	emptyQuery := ""

	// Empty query string is ignored — no LIKE clause should appear
	sqlMock.ExpectQuery(`SELECT \* FROM "consignments" WHERE trader_company_id`).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))

	result, err := svc.ListConsignments(ctx, Filter{TraderCompanyID: &companyID, Query: &emptyQuery})
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Empty(t, result.Items)
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

func TestConsignmentService_ListConsignments_NoFilter(t *testing.T) {
	svc := NewService(nil, nil, nil, nil, nil, nil)

	_, err := svc.ListConsignments(context.Background(), Filter{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "TraderCompanyID or CHACompanyID must be set")
}

func TestConsignmentService_ListConsignments_CHACompany(t *testing.T) {
	db, sqlMock := setupTestDB(t)
	svc := NewService(db, nil, nil, nil, nil, nil)
	ctx := context.Background()
	chaCompanyID := "cha-company-1"
	consignmentID := uuid.NewString()

	sqlMock.ExpectQuery(`(?i)SELECT .* FROM "consignments" WHERE cha_company_id`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "flow", "state", "trader_id", "trader_company_id", "cha_company_id", "created_at", "updated_at"}).
			AddRow(consignmentID, "EXPORT", "IN_PROGRESS", "trader1", "trader-co", chaCompanyID, time.Now(), time.Now()))

	result, err := svc.ListConsignments(ctx, Filter{CHACompanyID: &chaCompanyID})
	assert.NoError(t, err)
	assert.Equal(t, int64(1), result.Total)
	assert.Len(t, result.Items, 1)
	assert.Equal(t, consignmentID, result.Items[0].ID)
	assert.Equal(t, chaCompanyID, result.Items[0].ChaCompanyID)
	assert.NoError(t, sqlMock.ExpectationsWereMet())
}

func TestConsignmentService_ListConsignments_WithFilters(t *testing.T) {
	db, sqlMock := setupTestDB(t)
	svc := NewService(db, nil, nil, nil, nil, nil)
	ctx := context.Background()
	traderCompanyID := "trader-company-1"
	state := InProgress
	flow := FlowExport

	sqlMock.ExpectQuery(`(?i)SELECT .* FROM "consignments" WHERE trader_company_id`).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))

	result, err := svc.ListConsignments(ctx, Filter{
		TraderCompanyID: &traderCompanyID,
		State:           &state,
		Flow:            &flow,
	})
	assert.NoError(t, err)
	assert.Empty(t, result.Items)
	assert.NoError(t, sqlMock.ExpectationsWereMet())
}

func TestConsignmentService_GetConsignmentByID_NotFound(t *testing.T) {
	db, sqlMock := setupTestDB(t)
	svc := NewService(db, nil, nil, nil, nil, nil)

	id := uuid.NewString()
	sqlMock.ExpectQuery(`SELECT \* FROM "consignments" WHERE id = \$1 ORDER BY "consignments"."id" LIMIT \$2`).
		WithArgs(id, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))

	_, err := svc.GetConsignmentByID(context.Background(), id)
	assert.ErrorIs(t, err, ErrConsignmentNotFound)
}

func TestConsignmentService_CreateAndStartConsignment_Success(t *testing.T) {
	db, sqlMock := setupTestDB(t)
	mockUser := new(MockUserService)
	mockCompany := new(MockCompanyService)
	mockWM := new(MockWM)
	mockTaskStore := new(MockTaskStore)

	loader := &mockLoader{content: make(map[string][]byte)}
	reg := artifact.NewRegistry(loader)
	loader.content["workflows/trade-export-v1"] = []byte(`{"id":"trade-export-v1","name":"Trade Export V1"}`)
	reg.RegisterArtifact("trade-export-v1", "workflow", "", "workflows/trade-export-v1")

	svc := NewService(db, reg, nil, mockCompany, mockUser, mockTaskStore)
	require.NoError(t, svc.RegisterWorkflowManager(mockWM))

	traderID := "trader1"
	traderCompanyID := uuid.NewString()
	returnedID := uuid.NewString()

	mockUser.On("GetUser", mock.Anything, traderID).Return(&user.Record{ID: traderID, OUHandle: "trader-ou"}, nil)
	mockCompany.On("GetCompanyByOUHandle", mock.Anything, "trader-ou").Return(&company.Record{ID: traderCompanyID, Data: []byte(`{}`)}, nil)
	mockWM.On("StartWorkflow", mock.Anything, mock.AnythingOfType("string"), mock.Anything, mock.Anything).Return(nil)

	sqlMock.ExpectBegin()
	sqlMock.ExpectExec(`(?i)INSERT INTO "consignments"`).WillReturnResult(sqlmock.NewResult(1, 1))
	sqlMock.ExpectCommit()
	sqlMock.ExpectQuery(`(?i)SELECT .* FROM "consignments"`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "flow", "trader_id", "trader_company_id", "state", "created_at", "updated_at"}).
			AddRow(returnedID, "EXPORT", traderID, traderCompanyID, "IN_PROGRESS", time.Now(), time.Now()))
	mockTaskStore.On("GetAllTasks", mock.Anything, returnedID).Return([]store.TaskRecord(nil))

	result, err := svc.CreateAndStartConsignment(context.Background(), traderID)
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, returnedID, result.ID)
	assert.Equal(t, FlowExport, result.Flow)
	mockUser.AssertExpectations(t)
	mockCompany.AssertExpectations(t)
	mockWM.AssertExpectations(t)
	mockTaskStore.AssertExpectations(t)
	assert.NoError(t, sqlMock.ExpectationsWereMet())
}

func TestConsignmentService_CreateAndStartConsignment_UserLookupFails(t *testing.T) {
	svc := NewService(nil, nil, nil, nil, new(MockUserService), nil)
	mockUser := svc.userService.(*MockUserService)
	mockUser.On("GetUser", mock.Anything, "trader1").Return(nil, errors.New("user not found"))

	_, err := svc.CreateAndStartConsignment(context.Background(), "trader1")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "trader user lookup failed")
}

func TestConsignmentService_CreateAndStartConsignment_CompanyLookupFails(t *testing.T) {
	mockUser := new(MockUserService)
	mockCompany := new(MockCompanyService)
	svc := NewService(nil, nil, nil, mockCompany, mockUser, nil)

	mockUser.On("GetUser", mock.Anything, "trader1").Return(&user.Record{ID: "trader1", OUHandle: "trader-ou"}, nil)
	mockCompany.On("GetCompanyByOUHandle", mock.Anything, "trader-ou").Return(nil, errors.New("company not found"))

	_, err := svc.CreateAndStartConsignment(context.Background(), "trader1")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "trader company lookup failed")
}

func TestConsignmentService_CreateAndStartConsignment_StartWorkflowFails(t *testing.T) {
	db, sqlMock := setupTestDB(t)
	mockUser := new(MockUserService)
	mockCompany := new(MockCompanyService)
	mockWM := new(MockWM)

	loader := &mockLoader{content: make(map[string][]byte)}
	reg := artifact.NewRegistry(loader)
	loader.content["workflows/trade-export-v1"] = []byte(`{"id":"trade-export-v1","name":"Trade Export V1"}`)
	reg.RegisterArtifact("trade-export-v1", "workflow", "", "workflows/trade-export-v1")

	svc := NewService(db, reg, nil, mockCompany, mockUser, nil)
	require.NoError(t, svc.RegisterWorkflowManager(mockWM))

	mockUser.On("GetUser", mock.Anything, "trader1").Return(&user.Record{ID: "trader1", OUHandle: "trader-ou"}, nil)
	mockCompany.On("GetCompanyByOUHandle", mock.Anything, "trader-ou").Return(&company.Record{ID: "co1", Data: []byte(`{}`)}, nil)
	mockWM.On("StartWorkflow", mock.Anything, mock.AnythingOfType("string"), mock.Anything, mock.Anything).
		Return(errors.New("workflow engine unavailable"))

	sqlMock.ExpectBegin()
	sqlMock.ExpectExec(`(?i)INSERT INTO "consignments"`).WillReturnResult(sqlmock.NewResult(1, 1))
	sqlMock.ExpectRollback()

	_, err := svc.CreateAndStartConsignment(context.Background(), "trader1")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to register workflow")
	assert.NoError(t, sqlMock.ExpectationsWereMet())
}

func TestConsignmentService_GetConsignmentByID_DBError(t *testing.T) {
	db, sqlMock := setupTestDB(t)
	svc := NewService(db, nil, nil, nil, nil, nil)

	id := uuid.NewString()
	sqlMock.ExpectQuery(`SELECT \* FROM "consignments" WHERE id = \$1 ORDER BY "consignments"."id" LIMIT \$2`).
		WithArgs(id, 1).
		WillReturnError(errors.New("connection refused"))

	_, err := svc.GetConsignmentByID(context.Background(), id)
	assert.Error(t, err)
	assert.NotErrorIs(t, err, ErrConsignmentNotFound)
	assert.Contains(t, err.Error(), "failed to retrieve consignment")
}

func TestConsignmentService_OnWorkflowStatusChanged_Default(t *testing.T) {
	svc := NewService(nil, nil, nil, nil, nil, nil)
	err := svc.OnWorkflowStatusChanged(context.Background(), nil, "wf-1", WorkflowStatus("UNKNOWN"))
	assert.NoError(t, err)
}

func TestConsignmentService_buildNodeDTOsFromTaskRecords_NilStore(t *testing.T) {
	svc := NewService(nil, nil, nil, nil, nil, nil)
	_, err := svc.buildNodeDTOsFromTaskRecords(context.Background(), "id")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "task store not initialized")
}

func TestConsignmentService_buildNodeDTOsFromTaskRecords_WithTasks(t *testing.T) {
	mockTaskStore := new(MockTaskStore)
	svc := NewService(nil, nil, nil, nil, nil, mockTaskStore)
	ctx := context.Background()
	consignmentID := "cons-1"
	now := time.Now()

	tasks := []store.TaskRecord{
		{TaskID: "t1", TaskType: "FORM", State: "COMPLETED", ActiveTaskTemplateID: "step-1", CreatedAt: now, UpdatedAt: now},
		{TaskID: "t2", TaskType: "FORM", State: "FAILED", ActiveTaskTemplateID: "step-2", CreatedAt: now, UpdatedAt: now},
		{
			TaskID: "t3", TaskType: "FORM", State: "IN_PROGRESS", ActiveTaskTemplateID: "step-3", CreatedAt: now, UpdatedAt: now,
			RenderConfig: json.RawMessage(`{"title":"My Step"}`),
		},
		{TaskID: "t4", TaskType: "SYSTEM", State: "COMPLETED", ActiveTaskTemplateID: "sys", CreatedAt: now, UpdatedAt: now},
	}
	mockTaskStore.On("GetAllTasks", mock.Anything, consignmentID).Return(tasks)

	dtos, err := svc.buildNodeDTOsFromTaskRecords(ctx, consignmentID)
	assert.NoError(t, err)
	assert.Len(t, dtos, 3) // SYSTEM task filtered out
	assert.Equal(t, WorkflowNodeStateCompleted, dtos[0].State)
	assert.Equal(t, WorkflowNodeStateFailed, dtos[1].State)
	assert.Equal(t, WorkflowNodeStateInProgress, dtos[2].State)
	assert.Equal(t, "My Step", dtos[2].WorkflowNodeTemplate.Name)
}

func TestConsignmentService_GetConsignmentByID_BuildDTOError(t *testing.T) {
	// nil taskStore → buildNodeDTOsFromTaskRecords fails → buildConsignmentDetailDTO returns error.
	// Also covers the getWorkflowStatus no-wm-registered warning path (s.wm == nil).
	db, sqlMock := setupTestDB(t)
	svc := NewService(db, nil, nil, nil, nil, nil) // no taskStore, no wm
	id := uuid.NewString()

	sqlMock.ExpectQuery(`SELECT \* FROM "consignments" WHERE id = \$1 ORDER BY "consignments"."id" LIMIT \$2`).
		WithArgs(id, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "state"}).AddRow(id, "IN_PROGRESS"))

	_, err := svc.GetConsignmentByID(context.Background(), id)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "task store not initialized")
}

func TestConsignmentService_CompletionHandler_SaveFails(t *testing.T) {
	db, sqlMock := setupTestDB(t)
	svc := NewService(db, nil, nil, nil, nil, nil)
	id := uuid.NewString()

	sqlMock.ExpectQuery(`SELECT \* FROM "consignments" WHERE id = \$1`).
		WithArgs(id, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "state"}).AddRow(id, "IN_PROGRESS"))
	sqlMock.ExpectBegin()
	sqlMock.ExpectExec(`UPDATE "consignments"`).
		WillReturnError(errors.New("constraint violation"))
	sqlMock.ExpectRollback()

	err := svc.CompletionHandler(id, nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to update consignment")
	assert.NoError(t, sqlMock.ExpectationsWereMet())
}

func TestConsignmentService_startWorkflow_NoWM(t *testing.T) {
	svc := NewService(nil, nil, nil, nil, nil, nil)
	err := svc.startWorkflow(context.Background(), "id", workflow.WorkflowDefinition{}, nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no workflow manager registered")
}

func TestConsignmentService_getWorkflowStatus_NoWM(t *testing.T) {
	svc := NewService(nil, nil, nil, nil, nil, nil)
	err := svc.getWorkflowStatus(context.Background(), "id")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no workflow manager registered")
}

func TestTaskDisplayName(t *testing.T) {
	// nil render config → template ID
	assert.Equal(t, "trade-export-v1", taskDisplayName("trade-export-v1", nil))

	// root-level title present → use title
	rcRootTitle := json.RawMessage(`{"title":"My Root Title"}`)
	assert.Equal(t, "My Root Title", taskDisplayName("trade-export-v1", rcRootTitle))

	// root-level title empty → template ID
	rcEmptyTitle := json.RawMessage(`{"title":""}`)
	assert.Equal(t, "trade-export-v1", taskDisplayName("trade-export-v1", rcEmptyTitle))

	// root-level title missing → template ID
	rcNoTitle := json.RawMessage("{}")
	assert.Equal(t, "trade-export-v1", taskDisplayName("trade-export-v1", rcNoTitle))

	// JSON null render config → template ID
	rcNull := json.RawMessage("null")
	assert.Equal(t, "trade-export-v1", taskDisplayName("trade-export-v1", rcNull))

	// invalid JSON → template ID
	rc4 := json.RawMessage(`invalid`)
	assert.Equal(t, "trade-export-v1", taskDisplayName("trade-export-v1", rc4))
}
