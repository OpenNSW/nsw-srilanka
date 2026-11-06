package consignment

import (
	"context"
	"encoding/json"
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

	"github.com/OpenNSW/core/taskflow/store"

	nswaudit "github.com/OpenNSW/nsw-srilanka/internal/audit"
	"github.com/OpenNSW/nsw-srilanka/internal/authn"
	"github.com/OpenNSW/nsw-srilanka/internal/caseparty"
	"github.com/OpenNSW/nsw-srilanka/internal/profile/cha"
	"github.com/OpenNSW/nsw-srilanka/internal/profile/company"
	"github.com/OpenNSW/nsw-srilanka/internal/profile/user"
)

// testCatalogRoles satisfies validateRoles; tests unrelated to that check use it
// so NewRouter (and, from #272, NewService) always succeeds.
var testCatalogRoles = map[string]string{"trader": "Trader", "cha": "CHA"}

// mustNewRouter builds a Router with testCatalogRoles, failing the test
// immediately if construction errors.
func mustNewRouter(t *testing.T, cs *Service, chaService cha.Service, companyService company.Service, auditor nswaudit.Auditor) *Router {
	t.Helper()
	r, err := NewRouter(cs, chaService, companyService, auditor, testCatalogRoles)
	if err != nil {
		t.Fatalf("NewRouter: %v", err)
	}
	return r
}

func withAuthContext(ctx context.Context, userID string) context.Context {
	return authn.ContextWithPrincipal(ctx, &authn.Principal{
		Kind:   authn.KindUser,
		UserID: userID,
		Email:  userID + "@example.com",
	})
}

func withAuthContextOU(ctx context.Context, userID, ouHandle string) context.Context {
	return authn.ContextWithPrincipal(ctx, &authn.Principal{
		Kind:     authn.KindUser,
		UserID:   userID,
		Email:    userID + "@example.com",
		OUHandle: ouHandle,
	})
}

// withAuthContextRoles is withAuthContextOU plus the caller's JWT roles, for
// tests exercising role-entitlement checks.
func withAuthContextRoles(ctx context.Context, userID, ouHandle string, roles ...string) context.Context {
	return authn.ContextWithPrincipal(ctx, &authn.Principal{
		Kind:     authn.KindUser,
		UserID:   userID,
		Email:    userID + "@example.com",
		OUHandle: ouHandle,
		Roles:    roles,
	})
}

func TestConsignmentRouter_HandleGetConsignmentByID(t *testing.T) {
	db, sqlMock := setupTestDB(t)
	mockCompany := new(MockCompanyService)
	mockTaskStore := new(MockTaskStore)
	svc := mustNewService(t, db, nil, nil, mockCompany, nil, mockTaskStore)
	client, capture := nswaudit.NewWithCapture()
	r := mustNewRouter(t, svc, nil, mockCompany, client)

	consignmentID := uuid.NewString()
	companyID := "company-trader"
	mockCompany.On("GetCompanyByOUHandle", mock.Anything, "trader-ou").Return(&company.Record{ID: companyID, OUHandle: "trader-ou"}, nil)

	// A single consignment read: GetConsignmentByID enforces ownership on the row it reads.
	// The caller's company matches trader_company_id, so the DTO is built.
	sqlMock.ExpectQuery(`(?i)SELECT .* FROM "consignments"`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "state", "trader_company_id"}).AddRow(consignmentID, "IN_PROGRESS", companyID))

	mockTaskStore.On("GetTasks", mock.Anything, store.TaskFilter{CaseID: consignmentID}).Return(([]store.TaskRecord)(nil))

	req, _ := http.NewRequest("GET", "/api/v1/consignments/"+consignmentID, nil)
	req.SetPathValue("id", consignmentID)
	req = req.WithContext(withAuthContextRoles(req.Context(), "trader1", "trader-ou", "Trader"))

	w := httptest.NewRecorder()
	r.HandleGetConsignmentByID(w, req)
	assert.Equal(t, http.StatusOK, w.Code)

	recs := capture.Records()
	require.Len(t, recs, 1)
	assert.Equal(t, nswaudit.ActionRead, recs[0].Action)
	assert.Equal(t, nswaudit.StatusSuccess, recs[0].Status)
	assert.Equal(t, consignmentID, recs[0].TargetID)
	assert.Equal(t, companyID, recs[0].Metadata["callerCompanyId"])

	mockCompany.AssertExpectations(t)
	mockTaskStore.AssertExpectations(t)
}

// A CHA whose company is the consignment's cha_company_id may read it, even though it is
// not the trader company.
func TestConsignmentRouter_HandleGetConsignmentByID_SameCompanyCHA(t *testing.T) {
	db, sqlMock := setupTestDB(t)
	mockCompany := new(MockCompanyService)
	mockTaskStore := new(MockTaskStore)
	svc := mustNewService(t, db, nil, nil, mockCompany, nil, mockTaskStore)
	r := mustNewRouter(t, svc, nil, mockCompany, nil)

	consignmentID := uuid.NewString()
	chaCompanyID := "company-cha"
	mockCompany.On("GetCompanyByOUHandle", mock.Anything, "cha-ou").Return(&company.Record{ID: chaCompanyID, OUHandle: "cha-ou"}, nil)

	// Caller's company is the CHA (not the trader) company on the consignment row.
	sqlMock.ExpectQuery(`(?i)SELECT .* FROM "consignments"`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "state", "trader_company_id", "cha_company_id"}).
			AddRow(consignmentID, "IN_PROGRESS", "company-trader", chaCompanyID))

	mockTaskStore.On("GetTasks", mock.Anything, store.TaskFilter{CaseID: consignmentID}).Return(([]store.TaskRecord)(nil))

	req, _ := http.NewRequest("GET", "/api/v1/consignments/"+consignmentID, nil)
	req.SetPathValue("id", consignmentID)
	req = req.WithContext(withAuthContextRoles(req.Context(), "cha1", "cha-ou", "CHA"))

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
	client, capture := nswaudit.NewWithCapture()
	svc := mustNewService(t, db, nil, nil, mockCompany, nil, nil)
	r := mustNewRouter(t, svc, nil, mockCompany, client)

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

	recs := capture.Records()
	require.Len(t, recs, 1)
	assert.Equal(t, nswaudit.ActionRead, recs[0].Action)
	assert.Equal(t, nswaudit.TargetConsignment, recs[0].TargetType)
	assert.Equal(t, nswaudit.StatusFailure, recs[0].Status)
	assert.Equal(t, consignmentID, recs[0].TargetID)
}

// A caller with no resolvable company profile is denied (403) before any consignment read.
func TestConsignmentRouter_HandleGetConsignmentByID_CompanyNotFound(t *testing.T) {
	db, _ := setupTestDB(t)
	mockCompany := new(MockCompanyService)
	svc := mustNewService(t, db, nil, nil, mockCompany, nil, nil)
	r := mustNewRouter(t, svc, nil, mockCompany, nil)

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
	svc := mustNewService(t, db, nil, nil, mockCompany, nil, nil)
	r := mustNewRouter(t, svc, nil, mockCompany, nil)

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
	svc := mustNewService(t, db, nil, nil, mockCompany, nil, nil)
	r := mustNewRouter(t, svc, nil, mockCompany, nil)

	traderID := "trader1"
	companyID := "company-trader"
	mockCompany.On("GetCompanyByOUHandle", mock.Anything, "trader-ou").Return(&company.Record{ID: companyID, OUHandle: "trader-ou"}, nil)

	sqlMock.MatchExpectationsInOrder(false)
	sqlMock.ExpectQuery("(?i)SELECT count").WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	sqlMock.ExpectQuery("(?i)SELECT .* FROM \"consignments\"").WillReturnRows(sqlmock.NewRows([]string{"id", "trader_id", "trader_company_id"}).AddRow(uuid.NewString(), traderID, companyID))

	req, _ := http.NewRequest("GET", "/api/v1/consignments?state=IN_PROGRESS&flow=IMPORT", nil)
	req = req.WithContext(withAuthContextRoles(req.Context(), traderID, "trader-ou", "Trader"))
	w := httptest.NewRecorder()
	r.HandleGetConsignments(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
	mockCompany.AssertExpectations(t)
}

func TestConsignmentRouter_HandleGetConsignments_RejectsUnknownFilters(t *testing.T) {
	for _, query := range []string{"state=INITIALIZED", "state=FAILED", "flow=TRANSIT"} {
		t.Run(query, func(t *testing.T) {
			db, _ := setupTestDB(t)
			mockCompany := new(MockCompanyService)
			svc := mustNewService(t, db, nil, nil, mockCompany, nil, nil)
			r := mustNewRouter(t, svc, nil, mockCompany, nil)

			req, _ := http.NewRequest("GET", "/api/v1/consignments?"+query, nil)
			req = req.WithContext(withAuthContextRoles(req.Context(), "trader1", "trader-ou", "Trader"))
			w := httptest.NewRecorder()
			r.HandleGetConsignments(w, req)

			assert.Equal(t, http.StatusBadRequest, w.Code)
			mockCompany.AssertNotCalled(t, "GetCompanyByOUHandle", mock.Anything, mock.Anything)
		})
	}
}

func TestConsignmentRouter_HandleCreateConsignment_Success(t *testing.T) {
	db, sqlMock := setupTestDB(t)
	mockUser := new(MockUserService)
	mockCompany := new(MockCompanyService)
	mockTaskStore := new(MockTaskStore)

	svc := mustNewService(t, db, newExportRegistry(), nil, mockCompany, mockUser, mockTaskStore)
	svc.userFlows = userFlows
	require.NoError(t, svc.RegisterCaseFlows(&caseFlowsSpy{}))
	client, capture := nswaudit.NewWithCapture()
	r := mustNewRouter(t, svc, nil, mockCompany, client)

	traderID := "trader1"
	traderCompanyID := uuid.NewString()
	returnedID := uuid.NewString()

	mockUser.On("GetUser", mock.Anything, traderID).Return(&user.Record{ID: traderID, OUHandle: "trader-ou"}, nil)
	mockCompany.On("GetCompanyByOUHandle", mock.Anything, "trader-ou").Return(&company.Record{ID: traderCompanyID, OUHandle: "trader-ou", Data: []byte(`{}`)}, nil)

	expectCreateConsignment(sqlMock)
	sqlMock.ExpectQuery(`(?i)SELECT .* FROM "consignments"`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "flow", "trader_id", "trader_company_id", "state", "created_at", "updated_at"}).
			AddRow(returnedID, "EXPORT", traderID, traderCompanyID, "IN_PROGRESS", time.Now(), time.Now()))

	mockTaskStore.On("GetTasks", mock.Anything, store.TaskFilter{CaseID: returnedID}).Return([]store.TaskRecord(nil))

	req, _ := http.NewRequest("POST", "/api/v1/consignments", nil)
	req = req.WithContext(withAuthContextRoles(req.Context(), traderID, "", "Trader"))
	w := httptest.NewRecorder()
	r.HandleCreateConsignment(w, req)

	require.Equal(t, http.StatusCreated, w.Code)
	var detail DetailDTO
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &detail))
	assert.Equal(t, []FlowOption{}, detail.StartableFlows)

	recs := capture.Records()
	require.Len(t, recs, 1)
	assert.Equal(t, nswaudit.EventConsignment, recs[0].EventType)
	assert.Equal(t, nswaudit.ActionCreate, recs[0].Action)
	assert.Equal(t, nswaudit.TargetConsignment, recs[0].TargetType)
	assert.Equal(t, returnedID, recs[0].TargetID)
	assert.Equal(t, Flow("EXPORT"), recs[0].Metadata["flow"])
	assert.Equal(t, traderCompanyID, recs[0].Metadata["traderCompanyId"])

	mockUser.AssertExpectations(t)
	mockTaskStore.AssertExpectations(t)
}

func TestConsignmentRouter_HandleGetConsignments_WithSearch(t *testing.T) {
	db, sqlMock := setupTestDB(t)
	mockCompany := new(MockCompanyService)
	svc := mustNewService(t, db, nil, nil, mockCompany, nil, nil)
	r := mustNewRouter(t, svc, nil, mockCompany, nil)

	traderID := "trader1"
	companyID := "company-trader"
	mockCompany.On("GetCompanyByOUHandle", mock.Anything, "trader-ou").Return(&company.Record{ID: companyID, OUHandle: "trader-ou"}, nil)

	sqlMock.MatchExpectationsInOrder(false)
	sqlMock.ExpectQuery(`(?i)SELECT .* FROM "consignments".*LIKE`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "trader_id", "trader_company_id"}))

	req, _ := http.NewRequest("GET", "/api/v1/consignments?q=abc123", nil)
	req = req.WithContext(withAuthContextRoles(req.Context(), traderID, "trader-ou", "Trader"))
	w := httptest.NewRecorder()
	r.HandleGetConsignments(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
	mockCompany.AssertExpectations(t)
}

func TestConsignmentRouter_HandleCreateConsignment_Unauthorized(t *testing.T) {
	r := mustNewRouter(t, mustNewService(t, nil, nil, nil, nil, nil, nil), nil, nil, nil)

	req, _ := http.NewRequest("POST", "/api/v1/consignments", nil)
	w := httptest.NewRecorder()
	r.HandleCreateConsignment(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestConsignmentRouter_HandleGetConsignmentByID_NotFound(t *testing.T) {
	db, sqlMock := setupTestDB(t)
	mockCompany := new(MockCompanyService)
	svc := mustNewService(t, db, nil, nil, mockCompany, nil, nil)
	r := mustNewRouter(t, svc, nil, mockCompany, nil)

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
	r := mustNewRouter(t, mustNewService(t, nil, nil, nil, nil, nil, nil), nil, nil, nil)

	req, _ := http.NewRequest("GET", "/api/v1/consignments/", nil)
	req = req.WithContext(withAuthContext(req.Context(), "trader1"))
	w := httptest.NewRecorder()
	r.HandleGetConsignmentByID(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestConsignmentRouter_HandleGetConsignments_Unauthorized(t *testing.T) {
	r := mustNewRouter(t, mustNewService(t, nil, nil, nil, nil, nil, nil), nil, nil, nil)

	req, _ := http.NewRequest("GET", "/api/v1/consignments", nil)
	w := httptest.NewRecorder()
	r.HandleGetConsignments(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

// A caller holding both roles lists the consignments its company is the trader or the CHA on.
func TestConsignmentRouter_HandleGetConsignments_BothRoles(t *testing.T) {
	db, sqlMock := setupTestDB(t)
	mockCompany := new(MockCompanyService)
	svc := mustNewService(t, db, nil, nil, mockCompany, nil, nil)
	r := mustNewRouter(t, svc, nil, mockCompany, nil)

	mockCompany.On("GetCompanyByOUHandle", mock.Anything, "both-ou").
		Return(&company.Record{ID: "company-1"}, nil)
	sqlMock.ExpectQuery(consignmentSelect+`\(trader_company\.id = \$5 OR cha_company\.id = \$6\)`).
		WithArgs(partyRoleTrader, caseparty.TypeOU, partyRoleCHA, caseparty.TypeOU, "company-1", "company-1", sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))

	req, _ := http.NewRequest("GET", "/api/v1/consignments", nil)
	req = req.WithContext(withAuthContextRoles(req.Context(), "user1", "both-ou", "Trader", "CHA"))
	w := httptest.NewRecorder()
	r.HandleGetConsignments(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	mockCompany.AssertExpectations(t)
	require.NoError(t, sqlMock.ExpectationsWereMet())
}

func TestConsignmentRouter_HandleGetConsignments_CompanyNotFound(t *testing.T) {
	db, _ := setupTestDB(t)
	mockCompany := new(MockCompanyService)
	svc := mustNewService(t, db, nil, nil, mockCompany, nil, nil)
	client, capture := nswaudit.NewWithCapture()
	r := mustNewRouter(t, svc, nil, mockCompany, client)

	mockCompany.On("GetCompanyByOUHandle", mock.Anything, "trader-ou").
		Return(nil, company.ErrCompanyNotFound)

	req, _ := http.NewRequest("GET", "/api/v1/consignments", nil)
	req = req.WithContext(withAuthContextRoles(req.Context(), "trader1", "trader-ou", "Trader"))
	w := httptest.NewRecorder()
	r.HandleGetConsignments(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code)
	recs := capture.Records()
	require.Len(t, recs, 1)
	assert.Equal(t, nswaudit.StatusFailure, recs[0].Status)
	assert.Equal(t, "company not found", recs[0].Metadata["error"])
}

func TestConsignmentRouter_HandleGetConsignments_ListError(t *testing.T) {
	db, sqlMock := setupTestDB(t)
	mockCompany := new(MockCompanyService)
	svc := mustNewService(t, db, nil, nil, mockCompany, nil, nil)
	r := mustNewRouter(t, svc, nil, mockCompany, nil)

	mockCompany.On("GetCompanyByOUHandle", mock.Anything, "trader-ou").
		Return(&company.Record{ID: "company-1"}, nil)
	sqlMock.ExpectQuery(`(?i)SELECT .* FROM "consignments"`).
		WillReturnError(errors.New("db error"))

	req, _ := http.NewRequest("GET", "/api/v1/consignments", nil)
	req = req.WithContext(withAuthContextRoles(req.Context(), "trader1", "trader-ou", "Trader"))
	w := httptest.NewRecorder()
	r.HandleGetConsignments(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

// A CHA-only caller lists the consignments its company is the CHA on.
func TestConsignmentRouter_HandleGetConsignments_CHARole(t *testing.T) {
	db, sqlMock := setupTestDB(t)
	mockCompany := new(MockCompanyService)
	svc := mustNewService(t, db, nil, nil, mockCompany, nil, nil)
	r := mustNewRouter(t, svc, nil, mockCompany, nil)

	mockCompany.On("GetCompanyByOUHandle", mock.Anything, "cha-ou").
		Return(&company.Record{ID: "company-cha"}, nil)
	sqlMock.ExpectQuery(consignmentSelect+`cha_company\.id = \$5`).
		WithArgs(partyRoleTrader, caseparty.TypeOU, partyRoleCHA, caseparty.TypeOU, "company-cha", sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))

	req, _ := http.NewRequest("GET", "/api/v1/consignments", nil)
	req = req.WithContext(withAuthContextRoles(req.Context(), "cha1", "cha-ou", "CHA"))
	w := httptest.NewRecorder()
	r.HandleGetConsignments(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	mockCompany.AssertExpectations(t)
}

// A caller holding neither the trader nor the cha role is denied before any company
// lookup — the entitlement check must short-circuit ahead of it.
func TestConsignmentRouter_HandleGetConsignments_RoleNotHeld(t *testing.T) {
	mockCompany := new(MockCompanyService)
	client, capture := nswaudit.NewWithCapture()
	r := mustNewRouter(t, mustNewService(t, nil, nil, nil, nil, nil, nil), nil, mockCompany, client)

	req, _ := http.NewRequest("GET", "/api/v1/consignments", nil)
	req = req.WithContext(withAuthContextRoles(req.Context(), "officer1", "officer-ou", "NSW Admin"))
	w := httptest.NewRecorder()
	r.HandleGetConsignments(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code)
	mockCompany.AssertNotCalled(t, "GetCompanyByOUHandle", mock.Anything, mock.Anything)

	recs := capture.Records()
	require.Len(t, recs, 1)
	assert.Equal(t, nswaudit.ActionRead, recs[0].Action)
	assert.Equal(t, nswaudit.StatusFailure, recs[0].Status)
	assert.Equal(t, "role not held", recs[0].Metadata["error"])
}

// An empty/unusable OU handle surfaces as ErrInvalidCompanyID and must fail
// closed (403), not 500 — mirrors
// TestConsignmentRouter_HandleGetConsignmentByID_InvalidCompanyID.
func TestConsignmentRouter_HandleGetConsignments_InvalidCompanyID(t *testing.T) {
	mockCompany := new(MockCompanyService)
	svc := mustNewService(t, nil, nil, nil, mockCompany, nil, nil)
	client, capture := nswaudit.NewWithCapture()
	r := mustNewRouter(t, svc, nil, mockCompany, client)

	mockCompany.On("GetCompanyByOUHandle", mock.Anything, "").
		Return(nil, company.ErrInvalidCompanyID)

	req, _ := http.NewRequest("GET", "/api/v1/consignments", nil)
	req = req.WithContext(withAuthContextRoles(req.Context(), "trader1", "", "Trader")) // empty OU handle
	w := httptest.NewRecorder()
	r.HandleGetConsignments(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code)
	recs := capture.Records()
	require.Len(t, recs, 1)
	assert.Equal(t, nswaudit.StatusFailure, recs[0].Status)
	assert.Equal(t, "company not found", recs[0].Metadata["error"])
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
			_, err := NewRouter(nil, nil, nil, nil, tc.roles)
			if (err != nil) != tc.wantErr {
				t.Fatalf("NewRouter(...) err = %v, wantErr %v", err, tc.wantErr)
			}
		})
	}
}

// A caller without the trader role (here a CHA) cannot create a consignment.
func TestConsignmentRouter_HandleCreateConsignment_TraderRoleRequired(t *testing.T) {
	mockUser := new(MockUserService)
	svc := mustNewService(t, nil, nil, nil, nil, mockUser, nil)
	client, capture := nswaudit.NewWithCapture()
	r := mustNewRouter(t, svc, nil, nil, client)

	req, _ := http.NewRequest("POST", "/api/v1/consignments", nil)
	req = req.WithContext(withAuthContextRoles(req.Context(), "cha1", "cha-ou", "CHA"))
	w := httptest.NewRecorder()
	r.HandleCreateConsignment(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code)
	mockUser.AssertNotCalled(t, "GetUser", mock.Anything, mock.Anything)
	recs := capture.Records()
	require.Len(t, recs, 1)
	assert.Equal(t, nswaudit.ActionCreate, recs[0].Action)
	assert.Equal(t, nswaudit.StatusFailure, recs[0].Status)
	assert.Equal(t, "trader role not held", recs[0].Metadata["error"])
}

func TestConsignmentRouter_HandleCreateConsignment_ServiceError(t *testing.T) {
	mockUser := new(MockUserService)
	svc := mustNewService(t, nil, nil, nil, nil, mockUser, nil)
	r := mustNewRouter(t, svc, nil, nil, nil)

	mockUser.On("GetUser", mock.Anything, "trader1").Return(nil, errors.New("lookup failed"))

	req, _ := http.NewRequest("POST", "/api/v1/consignments", nil)
	req = req.WithContext(withAuthContextRoles(req.Context(), "trader1", "", "Trader"))
	w := httptest.NewRecorder()
	r.HandleCreateConsignment(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestConsignmentRouter_HandleGetConsignmentByID_ServiceError(t *testing.T) {
	db, sqlMock := setupTestDB(t)
	mockCompany := new(MockCompanyService)
	svc := mustNewService(t, db, nil, nil, mockCompany, nil, nil)
	r := mustNewRouter(t, svc, nil, mockCompany, nil)

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
