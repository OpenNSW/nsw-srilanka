package consignment

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/OpenNSW/core/artifact"
	"github.com/OpenNSW/core/authn"
	"github.com/OpenNSW/core/taskflow/store"
	workflow "github.com/OpenNSW/core/workflow"

	"github.com/OpenNSW/nsw-srilanka/internal/profile/company"
	"github.com/OpenNSW/nsw-srilanka/internal/profile/user"
)

func withAuthContext(ctx context.Context, userID string) context.Context {
	authCtx := &authn.AuthContext{
		User: &authn.UserContext{
			ID:    userID,
			Email: userID + "@example.com",
		},
	}
	return context.WithValue(ctx, authn.AuthContextKey, authCtx)
}

func withAuthContextOU(ctx context.Context, userID, ouHandle string) context.Context {
	authCtx := &authn.AuthContext{
		User: &authn.UserContext{
			ID:       userID,
			Email:    userID + "@example.com",
			OUHandle: ouHandle,
		},
	}
	return context.WithValue(ctx, authn.AuthContextKey, authCtx)
}

func TestConsignmentRouter_HandleGetConsignmentByID(t *testing.T) {
	db, sqlMock := setupTestDB(t)
	mockWM := new(MockWM)
	mockTaskStore := new(MockTaskStore)
	svc := NewService(db, nil, nil, nil, nil, mockTaskStore)
	require.NoError(t, svc.RegisterWorkflowManager(mockWM))
	r := NewRouter(svc, nil, nil)

	consignmentID := uuid.NewString()
	sqlMock.MatchExpectationsInOrder(false)
	sqlMock.ExpectQuery("(?i)SELECT .* FROM \"consignments\"").WillReturnRows(sqlmock.NewRows([]string{"id", "state"}).AddRow(consignmentID, "IN_PROGRESS"))

	mockWM.On("GetStatus", mock.Anything, consignmentID).Return((*workflow.WorkflowInstance)(nil), nil)
	mockTaskStore.On("GetAllTasks", mock.Anything, consignmentID).Return(([]store.TaskRecord)(nil))

	req, _ := http.NewRequest("GET", "/api/v1/consignments/"+consignmentID, nil)
	req.SetPathValue("id", consignmentID)
	req = req.WithContext(withAuthContext(req.Context(), "trader1"))

	w := httptest.NewRecorder()
	r.HandleGetConsignmentByID(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
	mockTaskStore.AssertExpectations(t)
}

func TestConsignmentRouter_HandleGetConsignments(t *testing.T) {
	db, sqlMock := setupTestDB(t)
	mockCompany := new(MockCompanyService)
	svc := NewService(db, nil, nil, mockCompany, nil, nil)
	r := NewRouter(svc, nil, mockCompany)

	traderID := "trader1"
	companyID := "company-trader"
	mockCompany.On("GetCompanyByOUHandle", mock.Anything, "trader-ou").Return(&company.Record{ID: companyID, OUHandle: "trader-ou"}, nil)

	sqlMock.MatchExpectationsInOrder(false)
	sqlMock.ExpectQuery("(?i)SELECT count").WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	sqlMock.ExpectQuery("(?i)SELECT .* FROM \"consignments\"").WillReturnRows(sqlmock.NewRows([]string{"id", "trader_id", "trader_company_id"}).AddRow(uuid.NewString(), traderID, companyID))

	req, _ := http.NewRequest("GET", "/api/v1/consignments?role=trader&state=IN_PROGRESS&flow=IMPORT", nil)
	req = req.WithContext(withAuthContextOU(req.Context(), traderID, "trader-ou"))
	w := httptest.NewRecorder()
	r.HandleGetConsignments(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
	mockCompany.AssertExpectations(t)
}

func TestConsignmentRouter_HandleCreateConsignment_Success(t *testing.T) {
	db, sqlMock := setupTestDB(t)
	mockUser := new(MockUserService)
	mockCompany := new(MockCompanyService)
	mockWM := new(MockWM)
	mockTaskStore := new(MockTaskStore)

	reg := artifact.NewRegistry()
	loader := &mockLoader{content: make(map[string][]byte)}
	reg.RegisterLoader("test", loader)
	loader.content["workflows/trade-export-v1"] = []byte(`{"id":"trade-export-v1","name":"Trade Export V1"}`)
	reg.RegisterArtifact("trade-export-v1", "workflow", "", "test", "workflows/trade-export-v1")

	svc := NewService(db, reg, nil, mockCompany, mockUser, mockTaskStore)
	require.NoError(t, svc.RegisterWorkflowManager(mockWM))
	r := NewRouter(svc, nil, mockCompany)

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

	req, _ := http.NewRequest("POST", "/api/v1/consignments", nil)
	req = req.WithContext(withAuthContext(req.Context(), traderID))
	w := httptest.NewRecorder()
	r.HandleCreateConsignment(w, req)

	assert.Equal(t, http.StatusCreated, w.Code)
	mockUser.AssertExpectations(t)
	mockWM.AssertExpectations(t)
	mockTaskStore.AssertExpectations(t)
}

func TestConsignmentRouter_HandleCreateConsignment_Unauthorized(t *testing.T) {
	r := NewRouter(NewService(nil, nil, nil, nil, nil, nil), nil, nil)

	req, _ := http.NewRequest("POST", "/api/v1/consignments", nil)
	w := httptest.NewRecorder()
	r.HandleCreateConsignment(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestConsignmentRouter_HandleGetConsignmentByID_NotFound(t *testing.T) {
	db, sqlMock := setupTestDB(t)
	mockWM := new(MockWM)
	mockTaskStore := new(MockTaskStore)
	svc := NewService(db, nil, nil, nil, nil, mockTaskStore)
	require.NoError(t, svc.RegisterWorkflowManager(mockWM))
	r := NewRouter(svc, nil, nil)

	id := uuid.NewString()
	sqlMock.ExpectQuery(`(?i)SELECT .* FROM "consignments"`).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))

	req, _ := http.NewRequest("GET", "/api/v1/consignments/"+id, nil)
	req.SetPathValue("id", id)
	req = req.WithContext(withAuthContext(req.Context(), "trader1"))
	w := httptest.NewRecorder()
	r.HandleGetConsignmentByID(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestConsignmentRouter_HandleGetConsignmentByID_MissingID(t *testing.T) {
	r := NewRouter(NewService(nil, nil, nil, nil, nil, nil), nil, nil)

	req, _ := http.NewRequest("GET", "/api/v1/consignments/", nil)
	req = req.WithContext(withAuthContext(req.Context(), "trader1"))
	w := httptest.NewRecorder()
	r.HandleGetConsignmentByID(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestConsignmentRouter_HandleGetConsignments_Unauthorized(t *testing.T) {
	r := NewRouter(NewService(nil, nil, nil, nil, nil, nil), nil, nil)

	req, _ := http.NewRequest("GET", "/api/v1/consignments", nil)
	w := httptest.NewRecorder()
	r.HandleGetConsignments(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestConsignmentRouter_HandleGetConsignments_InvalidRole(t *testing.T) {
	r := NewRouter(NewService(nil, nil, nil, nil, nil, nil), nil, nil)

	req, _ := http.NewRequest("GET", "/api/v1/consignments?role=superadmin", nil)
	req = req.WithContext(withAuthContextOU(req.Context(), "user1", "ou1"))
	w := httptest.NewRecorder()
	r.HandleGetConsignments(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestConsignmentRouter_HandleGetConsignments_DefaultRole(t *testing.T) {
	db, sqlMock := setupTestDB(t)
	mockCompany := new(MockCompanyService)
	svc := NewService(db, nil, nil, mockCompany, nil, nil)
	r := NewRouter(svc, nil, mockCompany)

	mockCompany.On("GetCompanyByOUHandle", mock.Anything, "trader-ou").
		Return(&company.Record{ID: "company-1"}, nil)
	sqlMock.ExpectQuery(`(?i)SELECT .* FROM "consignments"`).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))

	req, _ := http.NewRequest("GET", "/api/v1/consignments", nil) // no ?role param
	req = req.WithContext(withAuthContextOU(req.Context(), "trader1", "trader-ou"))
	w := httptest.NewRecorder()
	r.HandleGetConsignments(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	mockCompany.AssertExpectations(t)
}

func TestConsignmentRouter_HandleGetConsignments_CompanyNotFound(t *testing.T) {
	db, _ := setupTestDB(t)
	mockCompany := new(MockCompanyService)
	svc := NewService(db, nil, nil, mockCompany, nil, nil)
	r := NewRouter(svc, nil, mockCompany)

	mockCompany.On("GetCompanyByOUHandle", mock.Anything, "trader-ou").
		Return(nil, company.ErrCompanyNotFound)

	req, _ := http.NewRequest("GET", "/api/v1/consignments?role=trader", nil)
	req = req.WithContext(withAuthContextOU(req.Context(), "trader1", "trader-ou"))
	w := httptest.NewRecorder()
	r.HandleGetConsignments(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestConsignmentRouter_HandleGetConsignments_ListError(t *testing.T) {
	db, sqlMock := setupTestDB(t)
	mockCompany := new(MockCompanyService)
	svc := NewService(db, nil, nil, mockCompany, nil, nil)
	r := NewRouter(svc, nil, mockCompany)

	mockCompany.On("GetCompanyByOUHandle", mock.Anything, "trader-ou").
		Return(&company.Record{ID: "company-1"}, nil)
	sqlMock.ExpectQuery(`(?i)SELECT .* FROM "consignments"`).
		WillReturnError(errors.New("db error"))

	req, _ := http.NewRequest("GET", "/api/v1/consignments?role=trader", nil)
	req = req.WithContext(withAuthContextOU(req.Context(), "trader1", "trader-ou"))
	w := httptest.NewRecorder()
	r.HandleGetConsignments(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestConsignmentRouter_HandleCreateConsignment_ServiceError(t *testing.T) {
	mockUser := new(MockUserService)
	svc := NewService(nil, nil, nil, nil, mockUser, nil)
	r := NewRouter(svc, nil, nil)

	mockUser.On("GetUser", mock.Anything, "trader1").Return(nil, errors.New("lookup failed"))

	req, _ := http.NewRequest("POST", "/api/v1/consignments", nil)
	req = req.WithContext(withAuthContext(req.Context(), "trader1"))
	w := httptest.NewRecorder()
	r.HandleCreateConsignment(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestConsignmentRouter_HandleGetConsignmentByID_ServiceError(t *testing.T) {
	db, sqlMock := setupTestDB(t)
	svc := NewService(db, nil, nil, nil, nil, nil)
	r := NewRouter(svc, nil, nil)

	id := uuid.NewString()
	sqlMock.ExpectQuery(`(?i)SELECT .* FROM "consignments"`).
		WillReturnError(errors.New("connection refused"))

	req, _ := http.NewRequest("GET", "/api/v1/consignments/"+id, nil)
	req.SetPathValue("id", id)
	req = req.WithContext(withAuthContext(req.Context(), "trader1"))
	w := httptest.NewRecorder()
	r.HandleGetConsignmentByID(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}
