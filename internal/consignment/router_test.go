package consignment

import (
	"bytes"
	"context"
	"encoding/json"
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

	"github.com/OpenNSW/nsw/backend/srilanka/internal/profile/cha"
	"github.com/OpenNSW/nsw/backend/srilanka/internal/profile/company"
	"github.com/OpenNSW/nsw/backend/srilanka/internal/profile/user"
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
	sqlMock.ExpectQuery("(?i)SELECT .* FROM \"workflow_nodes\"").WillReturnRows(sqlmock.NewRows([]string{"workflow_id", "total", "completed"}).AddRow(uuid.NewString(), 1, 0))
	sqlMock.ExpectQuery("(?i)SELECT .* FROM \"workflows\"").WillReturnRows(sqlmock.NewRows([]string{"id", "end_node_id"}))

	req, _ := http.NewRequest("GET", "/api/v1/consignments?role=trader&state=IN_PROGRESS&flow=IMPORT", nil)
	req = req.WithContext(withAuthContextOU(req.Context(), traderID, "trader-ou"))
	w := httptest.NewRecorder()
	r.HandleGetConsignments(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
	mockCompany.AssertExpectations(t)
}

func TestConsignmentRouter_HandleCreateConsignment(t *testing.T) {
	db, sqlMock := setupTestDB(t)
	mockCompany := new(MockCompanyService)
	mockUser := new(MockUserService)
	mockTaskStore := new(MockTaskStore)
	svc := NewService(db, nil, nil, mockCompany, mockUser, mockTaskStore)
	r := NewRouter(svc, nil, mockCompany)

	traderID := "trader1"
	chaCompanyID := "cha-company"
	traderCompanyID := "trader-company"
	consignmentID := uuid.NewString()

	mockCompany.On("GetCompanyByID", mock.Anything, chaCompanyID).Return(&company.Record{ID: chaCompanyID, HasCHA: true}, nil)
	mockUser.On("GetUser", traderID).Return(&user.Record{ID: traderID, OUHandle: "trader-ou"}, nil)
	mockCompany.On("GetCompanyByOUHandle", mock.Anything, "trader-ou").Return(&company.Record{ID: traderCompanyID}, nil)

	payload := CreateConsignmentDTO{
		Flow:         FlowImport,
		ChaCompanyID: chaCompanyID,
	}
	body, _ := json.Marshal(payload)

	sqlMock.MatchExpectationsInOrder(false)
	sqlMock.ExpectBegin()
	sqlMock.ExpectExec("(?i)INSERT INTO \"consignments\"").
		WillReturnResult(sqlmock.NewResult(1, 1))
	sqlMock.ExpectCommit()

	sqlMock.ExpectQuery("(?i)SELECT .* FROM \"consignments\"").WillReturnRows(
		sqlmock.NewRows([]string{"id", "flow", "trader_id", "trader_company_id", "cha_company_id", "state"}).
			AddRow(consignmentID, string(FlowImport), traderID, traderCompanyID, chaCompanyID, string(Initialized)),
	)

	mockTaskStore.On("GetAllTasks", mock.Anything, consignmentID).Return(([]store.TaskRecord)(nil))

	req, _ := http.NewRequest("POST", "/api/v1/consignments", bytes.NewBuffer(body))
	req = req.WithContext(withAuthContextOU(req.Context(), traderID, "trader-ou"))
	w := httptest.NewRecorder()
	r.HandleCreateConsignment(w, req)
	assert.Equal(t, http.StatusCreated, w.Code)
	mockCompany.AssertExpectations(t)
	mockUser.AssertExpectations(t)
	mockTaskStore.AssertExpectations(t)
}

func TestConsignmentRouter_HandleInitializeConsignment(t *testing.T) {
	db, sqlMock := setupTestDB(t)
	mockWM := new(MockWM)
	mockCHA := new(MockCHAService)
	mockCompany := new(MockCompanyService)
	mockTaskStore := new(MockTaskStore)

	reg := artifact.NewRegistry()
	loader := &mockLoader{content: make(map[string][]byte)}
	reg.RegisterLoader("test", loader)
	wtID := "template1"
	loader.content["workflows/template1"] = []byte(`{"id":"template1","name":"Test Workflow"}`)
	reg.RegisterArtifact(wtID, "workflow", "", "test", "workflows/template1")

	svc := NewService(db, reg, mockCHA, mockCompany, nil, mockTaskStore)
	require.NoError(t, svc.RegisterWorkflowManager(mockWM))
	r := NewRouter(svc, mockCHA, mockCompany)

	id := uuid.NewString()
	chaID := "cha-1"
	chaCompanyID := "cha-company"
	traderCompanyID := "trader-company"
	chaEmail := "cha1@example.com"

	mockCHA.On("GetByEmail", mock.Anything, chaEmail).Return(&cha.Record{ID: chaID, CompanyID: chaCompanyID}, nil)
	mockCHA.On("GetByID", mock.Anything, chaID).Return(&cha.Record{ID: chaID, CompanyID: chaCompanyID}, nil)
	mockCompany.On("GetCompanyByID", mock.Anything, traderCompanyID).Return(&company.Record{ID: traderCompanyID, OUHandle: "trader-ou", Data: []byte(`{}`)}, nil)

	payload := InitializeConsignmentDTO{WorkflowTemplateID: wtID}
	body, _ := json.Marshal(payload)

	sqlMock.ExpectQuery("(?i)SELECT .* FROM \"consignments\"").WillReturnRows(sqlmock.NewRows([]string{"id", "state", "flow", "cha_company_id", "trader_company_id"}).AddRow(id, "INITIALIZED", "IMPORT", chaCompanyID, traderCompanyID))
	sqlMock.ExpectBegin()
	sqlMock.ExpectExec("(?i)UPDATE \"consignments\"").WillReturnResult(sqlmock.NewResult(1, 1))

	mockWM.On("StartWorkflow", mock.Anything, id, workflow.WorkflowDefinition{ID: "template1", Name: "Test Workflow"}, mock.Anything).Return(nil)
	sqlMock.ExpectCommit()

	sqlMock.ExpectQuery("(?i)SELECT .* FROM \"consignments\"").WillReturnRows(sqlmock.NewRows([]string{"id", "state", "created_at", "updated_at"}).AddRow(id, "IN_PROGRESS", time.Now(), time.Now()))
	mockWM.On("GetStatus", mock.Anything, id).Return(&workflow.WorkflowInstance{ID: id}, nil)

	mockTaskStore.On("GetAllTasks", mock.Anything, id).Return(([]store.TaskRecord)(nil))

	req, _ := http.NewRequest("PUT", "/api/v1/consignments/"+id, bytes.NewBuffer(body))
	req.SetPathValue("id", id)
	req = req.WithContext(withAuthContext(req.Context(), "cha1"))

	w := httptest.NewRecorder()
	r.HandleInitializeConsignment(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("w.Code = %d, body = %s", w.Code, w.Body.String())
	}
	assert.Equal(t, http.StatusOK, w.Code)
	mockCHA.AssertExpectations(t)
	mockCompany.AssertExpectations(t)
	mockTaskStore.AssertExpectations(t)
}
