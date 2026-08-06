package consignment

import (
	"context"
	"crypto"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	argus "github.com/LSFLK/argus/pkg/audit"
	"github.com/OpenNSW/core/artifact"
	"github.com/OpenNSW/core/authn"
	"github.com/OpenNSW/core/taskflow/store"
	workflow "github.com/OpenNSW/core/workflow"

	nswaudit "github.com/OpenNSW/nsw-srilanka/internal/audit"
	"github.com/OpenNSW/nsw-srilanka/internal/profile/cha"
	"github.com/OpenNSW/nsw-srilanka/internal/profile/company"
	"github.com/OpenNSW/nsw-srilanka/internal/profile/user"
)

// testCatalogRoles satisfies validateRoles; tests unrelated to that check use it
// so NewRouter (and, from #272, NewService) always succeeds.
var testCatalogRoles = map[string]string{"trader": "Trader", "cha": "CHA"}

// mustNewRouter builds a Router with testCatalogRoles, failing the test
// immediately if construction errors.
func mustNewRouter(t *testing.T, cs *Service, chaService cha.Service, companyService company.Service, recorder *nswaudit.Recorder) *Router {
	t.Helper()
	r, err := NewRouter(cs, chaService, companyService, recorder, testCatalogRoles)
	if err != nil {
		t.Fatalf("NewRouter: %v", err)
	}
	return r
}

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

// withAuthContextRoles is withAuthContextOU plus the caller's JWT roles, for
// tests exercising role-entitlement checks.
func withAuthContextRoles(ctx context.Context, userID, ouHandle string, roles ...string) context.Context {
	authCtx := &authn.AuthContext{
		User: &authn.UserContext{
			ID:       userID,
			Email:    userID + "@example.com",
			OUHandle: ouHandle,
			Roles:    roles,
		},
	}
	return context.WithValue(ctx, authn.AuthContextKey, authCtx)
}

func TestConsignmentRouter_HandleGetConsignmentByID(t *testing.T) {
	db, sqlMock := setupTestDB(t)
	mockCompany := new(MockCompanyService)
	mockWM := new(MockWM)
	mockTaskStore := new(MockTaskStore)
	svc := NewService(db, nil, nil, mockCompany, nil, mockTaskStore)
	require.NoError(t, svc.RegisterWorkflowManager(mockWM))
	r := mustNewRouter(t, svc, nil, mockCompany, nswaudit.NewRecorder(nil))

	consignmentID := uuid.NewString()
	companyID := "company-trader"
	mockCompany.On("GetCompanyByOUHandle", mock.Anything, "trader-ou").Return(&company.Record{ID: companyID, OUHandle: "trader-ou"}, nil)

	// A single consignment read: GetConsignmentByID enforces ownership on the row it reads.
	// The caller's company matches trader_company_id, so the DTO is built.
	sqlMock.ExpectQuery(`(?i)SELECT .* FROM "consignments"`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "state", "trader_company_id"}).AddRow(consignmentID, "IN_PROGRESS", companyID))

	mockWM.On("GetStatus", mock.Anything, consignmentID).Return((*workflow.WorkflowInstance)(nil), nil)
	mockTaskStore.On("GetAllTasks", mock.Anything, consignmentID).Return(([]store.TaskRecord)(nil))

	req, _ := http.NewRequest("GET", "/api/v1/consignments/"+consignmentID, nil)
	req.SetPathValue("id", consignmentID)
	req = req.WithContext(withAuthContextOU(req.Context(), "trader1", "trader-ou"))

	w := httptest.NewRecorder()
	r.HandleGetConsignmentByID(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
	mockCompany.AssertExpectations(t)
	mockTaskStore.AssertExpectations(t)
}

// A CHA whose company is the consignment's cha_company_id may read it, even though it is
// not the trader company.
func TestConsignmentRouter_HandleGetConsignmentByID_SameCompanyCHA(t *testing.T) {
	db, sqlMock := setupTestDB(t)
	mockCompany := new(MockCompanyService)
	mockWM := new(MockWM)
	mockTaskStore := new(MockTaskStore)
	svc := NewService(db, nil, nil, mockCompany, nil, mockTaskStore)
	require.NoError(t, svc.RegisterWorkflowManager(mockWM))
	r := mustNewRouter(t, svc, nil, mockCompany, nswaudit.NewRecorder(nil))

	consignmentID := uuid.NewString()
	chaCompanyID := "company-cha"
	mockCompany.On("GetCompanyByOUHandle", mock.Anything, "cha-ou").Return(&company.Record{ID: chaCompanyID, OUHandle: "cha-ou"}, nil)

	// Caller's company is the CHA (not the trader) company on the consignment row.
	sqlMock.ExpectQuery(`(?i)SELECT .* FROM "consignments"`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "state", "trader_company_id", "cha_company_id"}).
			AddRow(consignmentID, "IN_PROGRESS", "company-trader", chaCompanyID))

	mockWM.On("GetStatus", mock.Anything, consignmentID).Return((*workflow.WorkflowInstance)(nil), nil)
	mockTaskStore.On("GetAllTasks", mock.Anything, consignmentID).Return(([]store.TaskRecord)(nil))

	req, _ := http.NewRequest("GET", "/api/v1/consignments/"+consignmentID, nil)
	req.SetPathValue("id", consignmentID)
	req = req.WithContext(withAuthContextOU(req.Context(), "cha1", "cha-ou"))

	w := httptest.NewRecorder()
	r.HandleGetConsignmentByID(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
	mockCompany.AssertExpectations(t)
}

// A caller whose company is neither the trader nor the CHA is denied with 404 (not 403, to
// avoid an existence oracle), the DTO is never built, and the denial is audited.
func TestConsignmentRouter_HandleGetConsignmentByID_DifferentCompany(t *testing.T) {
	db, sqlMock := setupTestDB(t)
	mockCompany := new(MockCompanyService)
	auditor := &mockAuditor{}
	svc := NewService(db, nil, nil, mockCompany, nil, nil)
	r := mustNewRouter(t, svc, nil, mockCompany, nswaudit.NewRecorder(auditor))

	consignmentID := uuid.NewString()
	mockCompany.On("GetCompanyByOUHandle", mock.Anything, "outsider-ou").Return(&company.Record{ID: "company-outsider", OUHandle: "outsider-ou"}, nil)

	// Only the ownership lookup runs; no workflow-engine / task-store calls on the denial path.
	sqlMock.ExpectQuery(`(?i)SELECT .* FROM "consignments"`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "state", "trader_company_id", "cha_company_id"}).
			AddRow(consignmentID, "IN_PROGRESS", "company-trader", "company-cha"))

	req, _ := http.NewRequest("GET", "/api/v1/consignments/"+consignmentID, nil)
	req.SetPathValue("id", consignmentID)
	req = req.WithContext(withAuthContextOU(req.Context(), "outsider1", "outsider-ou"))

	w := httptest.NewRecorder()
	r.HandleGetConsignmentByID(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)

	require.Len(t, auditor.events, 1)
	assert.Equal(t, string(nswaudit.ActionRead), auditor.events[0].Action)
	assert.Equal(t, string(nswaudit.TargetConsignment), auditor.events[0].TargetType)
	assert.Equal(t, argus.StatusFailure, auditor.events[0].Status)
	require.NotNil(t, auditor.events[0].TargetID)
	assert.Equal(t, consignmentID, *auditor.events[0].TargetID)
}

// A caller with no resolvable company profile is denied (403) before any consignment read.
func TestConsignmentRouter_HandleGetConsignmentByID_CompanyNotFound(t *testing.T) {
	db, _ := setupTestDB(t)
	mockCompany := new(MockCompanyService)
	svc := NewService(db, nil, nil, mockCompany, nil, nil)
	r := mustNewRouter(t, svc, nil, mockCompany, nswaudit.NewRecorder(nil))

	id := uuid.NewString()
	mockCompany.On("GetCompanyByOUHandle", mock.Anything, "trader-ou").
		Return(nil, company.ErrCompanyNotFound)

	req, _ := http.NewRequest("GET", "/api/v1/consignments/"+id, nil)
	req.SetPathValue("id", id)
	req = req.WithContext(withAuthContextOU(req.Context(), "trader1", "trader-ou"))
	w := httptest.NewRecorder()
	r.HandleGetConsignmentByID(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code)
}

// An empty/unusable OU handle surfaces as ErrInvalidCompanyID and must fail closed (403), not 500.
func TestConsignmentRouter_HandleGetConsignmentByID_InvalidCompanyID(t *testing.T) {
	db, _ := setupTestDB(t)
	mockCompany := new(MockCompanyService)
	svc := NewService(db, nil, nil, mockCompany, nil, nil)
	r := mustNewRouter(t, svc, nil, mockCompany, nswaudit.NewRecorder(nil))

	id := uuid.NewString()
	mockCompany.On("GetCompanyByOUHandle", mock.Anything, "").
		Return(nil, company.ErrInvalidCompanyID)

	req, _ := http.NewRequest("GET", "/api/v1/consignments/"+id, nil)
	req.SetPathValue("id", id)
	req = req.WithContext(withAuthContext(req.Context(), "trader1")) // withAuthContext leaves OUHandle empty
	w := httptest.NewRecorder()
	r.HandleGetConsignmentByID(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestConsignmentRouter_HandleGetConsignments(t *testing.T) {
	db, sqlMock := setupTestDB(t)
	mockCompany := new(MockCompanyService)
	svc := NewService(db, nil, nil, mockCompany, nil, nil)
	r := mustNewRouter(t, svc, nil, mockCompany, nswaudit.NewRecorder(nil))

	traderID := "trader1"
	companyID := "company-trader"
	mockCompany.On("GetCompanyByOUHandle", mock.Anything, "trader-ou").Return(&company.Record{ID: companyID, OUHandle: "trader-ou"}, nil)

	sqlMock.MatchExpectationsInOrder(false)
	sqlMock.ExpectQuery("(?i)SELECT count").WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	sqlMock.ExpectQuery("(?i)SELECT .* FROM \"consignments\"").WillReturnRows(sqlmock.NewRows([]string{"id", "trader_id", "trader_company_id"}).AddRow(uuid.NewString(), traderID, companyID))

	req, _ := http.NewRequest("GET", "/api/v1/consignments?role=trader&state=IN_PROGRESS&flow=IMPORT", nil)
	req = req.WithContext(withAuthContextRoles(req.Context(), traderID, "trader-ou", "Trader"))
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

	loader := &mockLoader{content: make(map[string][]byte)}
	reg := artifact.NewRegistry(loader)
	loader.content["workflows/trade-export-v1"] = []byte(`{"id":"trade-export-v1","name":"Trade Export V1"}`)
	reg.RegisterArtifact("trade-export-v1", "workflow", "", "workflows/trade-export-v1")

	svc := NewService(db, reg, nil, mockCompany, mockUser, mockTaskStore)
	require.NoError(t, svc.RegisterWorkflowManager(mockWM))
	auditor := &mockAuditor{}
	r := mustNewRouter(t, svc, nil, mockCompany, nswaudit.NewRecorder(auditor))

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

	// Assert audit event was recorded
	require.Len(t, auditor.events, 1)
	assert.Equal(t, string(nswaudit.EventConsignment), auditor.events[0].EventType)
	assert.Equal(t, string(nswaudit.ActionCreate), auditor.events[0].Action)
	assert.Equal(t, string(nswaudit.TargetConsignment), auditor.events[0].TargetType)
	assert.Equal(t, returnedID, *auditor.events[0].TargetID)
	assert.Equal(t, Flow("EXPORT"), auditor.events[0].Metadata["flow"])
	assert.Equal(t, traderCompanyID, auditor.events[0].Metadata["traderCompanyId"])

	mockUser.AssertExpectations(t)
	mockWM.AssertExpectations(t)
	mockTaskStore.AssertExpectations(t)
}

func TestConsignmentRouter_HandleGetConsignments_WithSearch(t *testing.T) {
	db, sqlMock := setupTestDB(t)
	mockCompany := new(MockCompanyService)
	svc := NewService(db, nil, nil, mockCompany, nil, nil)
	r := mustNewRouter(t, svc, nil, mockCompany, nswaudit.NewRecorder(nil))

	traderID := "trader1"
	companyID := "company-trader"
	mockCompany.On("GetCompanyByOUHandle", mock.Anything, "trader-ou").Return(&company.Record{ID: companyID, OUHandle: "trader-ou"}, nil)

	sqlMock.MatchExpectationsInOrder(false)
	sqlMock.ExpectQuery(`(?i)SELECT .* FROM "consignments".*LIKE`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "trader_id", "trader_company_id"}))

	req, _ := http.NewRequest("GET", "/api/v1/consignments?role=trader&q=abc123", nil)
	req = req.WithContext(withAuthContextRoles(req.Context(), traderID, "trader-ou", "Trader"))
	w := httptest.NewRecorder()
	r.HandleGetConsignments(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
	mockCompany.AssertExpectations(t)
}

func TestConsignmentRouter_HandleCreateConsignment_Unauthorized(t *testing.T) {
	r := mustNewRouter(t, NewService(nil, nil, nil, nil, nil, nil), nil, nil, nswaudit.NewRecorder(nil))

	req, _ := http.NewRequest("POST", "/api/v1/consignments", nil)
	w := httptest.NewRecorder()
	r.HandleCreateConsignment(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestConsignmentRouter_HandleGetConsignmentByID_NotFound(t *testing.T) {
	db, sqlMock := setupTestDB(t)
	mockCompany := new(MockCompanyService)
	svc := NewService(db, nil, nil, mockCompany, nil, nil)
	r := mustNewRouter(t, svc, nil, mockCompany, nswaudit.NewRecorder(nil))

	id := uuid.NewString()
	mockCompany.On("GetCompanyByOUHandle", mock.Anything, "trader-ou").Return(&company.Record{ID: "company-1"}, nil)
	// The ownership lookup (first DB touch) finds no row -> 404.
	sqlMock.ExpectQuery(`(?i)SELECT .* FROM "consignments"`).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))

	req, _ := http.NewRequest("GET", "/api/v1/consignments/"+id, nil)
	req.SetPathValue("id", id)
	req = req.WithContext(withAuthContextOU(req.Context(), "trader1", "trader-ou"))
	w := httptest.NewRecorder()
	r.HandleGetConsignmentByID(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestConsignmentRouter_HandleGetConsignmentByID_MissingID(t *testing.T) {
	r := mustNewRouter(t, NewService(nil, nil, nil, nil, nil, nil), nil, nil, nswaudit.NewRecorder(nil))

	req, _ := http.NewRequest("GET", "/api/v1/consignments/", nil)
	req = req.WithContext(withAuthContext(req.Context(), "trader1"))
	w := httptest.NewRecorder()
	r.HandleGetConsignmentByID(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestConsignmentRouter_HandleGetConsignments_Unauthorized(t *testing.T) {
	r := mustNewRouter(t, NewService(nil, nil, nil, nil, nil, nil), nil, nil, nswaudit.NewRecorder(nil))

	req, _ := http.NewRequest("GET", "/api/v1/consignments", nil)
	w := httptest.NewRecorder()
	r.HandleGetConsignments(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestConsignmentRouter_HandleGetConsignments_InvalidRole(t *testing.T) {
	r := mustNewRouter(t, NewService(nil, nil, nil, nil, nil, nil), nil, nil, nswaudit.NewRecorder(nil))

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
	r := mustNewRouter(t, svc, nil, mockCompany, nswaudit.NewRecorder(nil))

	mockCompany.On("GetCompanyByOUHandle", mock.Anything, "trader-ou").
		Return(&company.Record{ID: "company-1"}, nil)
	sqlMock.ExpectQuery(`(?i)SELECT .* FROM "consignments"`).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))

	req, _ := http.NewRequest("GET", "/api/v1/consignments", nil) // no ?role param
	req = req.WithContext(withAuthContextRoles(req.Context(), "trader1", "trader-ou", "Trader"))
	w := httptest.NewRecorder()
	r.HandleGetConsignments(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	mockCompany.AssertExpectations(t)
}

func TestConsignmentRouter_HandleGetConsignments_CompanyNotFound(t *testing.T) {
	db, _ := setupTestDB(t)
	mockCompany := new(MockCompanyService)
	svc := NewService(db, nil, nil, mockCompany, nil, nil)
	r := mustNewRouter(t, svc, nil, mockCompany, nswaudit.NewRecorder(nil))

	mockCompany.On("GetCompanyByOUHandle", mock.Anything, "trader-ou").
		Return(nil, company.ErrCompanyNotFound)

	req, _ := http.NewRequest("GET", "/api/v1/consignments?role=trader", nil)
	req = req.WithContext(withAuthContextRoles(req.Context(), "trader1", "trader-ou", "Trader"))
	w := httptest.NewRecorder()
	r.HandleGetConsignments(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestConsignmentRouter_HandleGetConsignments_ListError(t *testing.T) {
	db, sqlMock := setupTestDB(t)
	mockCompany := new(MockCompanyService)
	svc := NewService(db, nil, nil, mockCompany, nil, nil)
	r := mustNewRouter(t, svc, nil, mockCompany, nswaudit.NewRecorder(nil))

	mockCompany.On("GetCompanyByOUHandle", mock.Anything, "trader-ou").
		Return(&company.Record{ID: "company-1"}, nil)
	sqlMock.ExpectQuery(`(?i)SELECT .* FROM "consignments"`).
		WillReturnError(errors.New("db error"))

	req, _ := http.NewRequest("GET", "/api/v1/consignments?role=trader", nil)
	req = req.WithContext(withAuthContextRoles(req.Context(), "trader1", "trader-ou", "Trader"))
	w := httptest.NewRecorder()
	r.HandleGetConsignments(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

// A CHA-role caller requesting role=cha is allowed — there is no coverage of the
// cha branch elsewhere in this file.
func TestConsignmentRouter_HandleGetConsignments_CHARole(t *testing.T) {
	db, sqlMock := setupTestDB(t)
	mockCompany := new(MockCompanyService)
	svc := NewService(db, nil, nil, mockCompany, nil, nil)
	r := mustNewRouter(t, svc, nil, mockCompany, nswaudit.NewRecorder(nil))

	mockCompany.On("GetCompanyByOUHandle", mock.Anything, "cha-ou").
		Return(&company.Record{ID: "company-cha"}, nil)
	sqlMock.ExpectQuery(`(?i)SELECT .* FROM "consignments"`).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))

	req, _ := http.NewRequest("GET", "/api/v1/consignments?role=cha", nil)
	req = req.WithContext(withAuthContextRoles(req.Context(), "cha1", "cha-ou", "CHA"))
	w := httptest.NewRecorder()
	r.HandleGetConsignments(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	mockCompany.AssertExpectations(t)
}

// A Trader-only caller asserting role=cha via the query param is denied before
// any company lookup — the entitlement check must short-circuit ahead of it.
func TestConsignmentRouter_HandleGetConsignments_RoleNotHeld(t *testing.T) {
	mockCompany := new(MockCompanyService)
	r := mustNewRouter(t, NewService(nil, nil, nil, nil, nil, nil), nil, mockCompany, nswaudit.NewRecorder(nil))

	req, _ := http.NewRequest("GET", "/api/v1/consignments?role=cha", nil)
	req = req.WithContext(withAuthContextRoles(req.Context(), "trader1", "trader-ou", "Trader"))
	w := httptest.NewRecorder()
	r.HandleGetConsignments(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code)
	mockCompany.AssertNotCalled(t, "GetCompanyByOUHandle", mock.Anything, mock.Anything)
}

// An empty/unusable OU handle surfaces as ErrInvalidCompanyID and must fail
// closed (403), not 500 — mirrors
// TestConsignmentRouter_HandleGetConsignmentByID_InvalidCompanyID.
func TestConsignmentRouter_HandleGetConsignments_InvalidCompanyID(t *testing.T) {
	mockCompany := new(MockCompanyService)
	svc := NewService(nil, nil, nil, mockCompany, nil, nil)
	r := mustNewRouter(t, svc, nil, mockCompany, nswaudit.NewRecorder(nil))

	mockCompany.On("GetCompanyByOUHandle", mock.Anything, "").
		Return(nil, company.ErrInvalidCompanyID)

	req, _ := http.NewRequest("GET", "/api/v1/consignments?role=trader", nil)
	req = req.WithContext(withAuthContextRoles(req.Context(), "trader1", "", "Trader")) // empty OU handle
	w := httptest.NewRecorder()
	r.HandleGetConsignments(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestNewRouter_ValidatesRoles(t *testing.T) {
	tests := []struct {
		name    string
		roles   map[string]string
		wantErr bool
	}{
		{name: "both present", roles: map[string]string{"trader": "Trader", "cha": "CHA"}},
		{name: "extra roles ignored", roles: map[string]string{"trader": "Trader", "cha": "CHA", "fcau": "FCAU_TO_NSW"}},
		{name: "missing cha", roles: map[string]string{"trader": "Trader"}, wantErr: true},
		{name: "missing trader", roles: map[string]string{"cha": "CHA"}, wantErr: true},
		{name: "cha present but empty", roles: map[string]string{"trader": "Trader", "cha": ""}, wantErr: true},
		{name: "missing both", roles: map[string]string{}, wantErr: true},
		{name: "nil map", roles: nil, wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := NewRouter(nil, nil, nil, nswaudit.NewRecorder(nil), tc.roles)
			if (err != nil) != tc.wantErr {
				t.Fatalf("NewRouter(...) err = %v, wantErr %v", err, tc.wantErr)
			}
		})
	}
}

func TestConsignmentRouter_HandleCreateConsignment_ServiceError(t *testing.T) {
	mockUser := new(MockUserService)
	svc := NewService(nil, nil, nil, nil, mockUser, nil)
	r := mustNewRouter(t, svc, nil, nil, nswaudit.NewRecorder(nil))

	mockUser.On("GetUser", mock.Anything, "trader1").Return(nil, errors.New("lookup failed"))

	req, _ := http.NewRequest("POST", "/api/v1/consignments", nil)
	req = req.WithContext(withAuthContext(req.Context(), "trader1"))
	w := httptest.NewRecorder()
	r.HandleCreateConsignment(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestConsignmentRouter_HandleGetConsignmentByID_ServiceError(t *testing.T) {
	db, sqlMock := setupTestDB(t)
	mockCompany := new(MockCompanyService)
	svc := NewService(db, nil, nil, mockCompany, nil, nil)
	r := mustNewRouter(t, svc, nil, mockCompany, nswaudit.NewRecorder(nil))

	id := uuid.NewString()
	mockCompany.On("GetCompanyByOUHandle", mock.Anything, "trader-ou").Return(&company.Record{ID: "company-1"}, nil)
	// The ownership lookup (first DB touch) errors -> 500.
	sqlMock.ExpectQuery(`(?i)SELECT .* FROM "consignments"`).
		WillReturnError(errors.New("connection refused"))

	req, _ := http.NewRequest("GET", "/api/v1/consignments/"+id, nil)
	req.SetPathValue("id", id)
	req = req.WithContext(withAuthContextOU(req.Context(), "trader1", "trader-ou"))
	w := httptest.NewRecorder()
	r.HandleGetConsignmentByID(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

type mockAuditor struct {
	mu     sync.Mutex
	events []*argus.AuditLogRequest
}

func (m *mockAuditor) LogEvent(ctx context.Context, event *argus.AuditLogRequest) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.events = append(m.events, event)
	return true
}

func (m *mockAuditor) IsEnabled() bool { return true }

func (m *mockAuditor) SignEvent(ctx context.Context, event *argus.AuditLogRequest) error {
	return nil
}

func (m *mockAuditor) SignMessageBytes(ctx context.Context, message []byte) (string, error) {
	return "", nil
}

func (m *mockAuditor) LogSignedEvent(ctx context.Context, event *argus.AuditLogRequest) {}

func (m *mockAuditor) VerifyIntegrity(event *argus.AuditLogRequest, publicKey crypto.PublicKey) (bool, error) {
	return true, nil
}

func (m *mockAuditor) Close(ctx context.Context) error {
	return nil
}
