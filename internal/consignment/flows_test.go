package consignment

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/OpenNSW/core/taskflow/store"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	nswaudit "github.com/OpenNSW/nsw-srilanka/internal/audit"
	"github.com/OpenNSW/nsw-srilanka/internal/caseflow"
	"github.com/OpenNSW/nsw-srilanka/internal/profile/company"
)

var auxFlow = FlowOption{TemplateID: "aux_flow", Label: "Auxiliary flow"}

// userFlows are the flows newFlowsService offers: aux_flow to a trader or CHA, and
// cha_flow to a CHA only.
var userFlows = caseflow.UserFlows{Enabled: true, Workflows: []caseflow.UserFlow{
	{ID: "aux_flow", Label: "Auxiliary flow", Roles: []string{"trader", "cha"}},
	{ID: "cha_flow", Label: "CHA flow", Roles: []string{"cha"}},
}}

// newFlowsService is a Service offering userFlows, with flows recording every start.
func newFlowsService(t *testing.T) (*Service, sqlmock.Sqlmock, *caseFlowsSpy) {
	t.Helper()
	db, sqlMock := setupTestDB(t)
	companies := new(MockCompanyService)
	companies.On("GetCompanyByID", mock.Anything, mock.Anything).
		Return(&company.Record{ID: "co-trader", Name: "Trader Co", Data: []byte(`{}`)}, nil).Maybe()
	svc := mustNewService(t, db, newExportRegistry(), nil, companies, nil, nil)
	svc.userFlows = userFlows
	flows := &caseFlowsSpy{}
	require.NoError(t, svc.RegisterCaseFlows(flows))
	return svc, sqlMock, flows
}

// expectConsignment answers the next consignment read with a row owned by traderCompanyID.
func expectConsignment(sqlMock sqlmock.Sqlmock, id, traderCompanyID, state string) {
	sqlMock.ExpectQuery(`(?i)SELECT .* FROM "consignments" JOIN cases`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "state", "trader_company_id", "created_at", "updated_at"}).
			AddRow(id, state, traderCompanyID, time.Now(), time.Now()))
}

func TestStartConsignmentFlow(t *testing.T) {
	t.Run("starts a startable flow, finished consignment included", func(t *testing.T) {
		svc, sqlMock, flows := newFlowsService(t)
		id := uuid.NewString()
		expectConsignment(sqlMock, id, "co-trader", "FINISHED")

		got, err := svc.StartConsignmentFlow(context.Background(), id, "aux_flow", "co-trader", []string{"Trader"})
		require.NoError(t, err)
		assert.Equal(t, "aux_flow", got.TemplateID)

		require.Len(t, flows.reqs, 1)
		req := flows.reqs[0]
		assert.Equal(t, id, req.CaseID)
		assert.Equal(t, "aux_flow", req.TemplateID)
		assert.Empty(t, req.WorkflowID)
		// Every variable is set by the server, from the consignment: the consignment ID and
		// the trader company, as the export flow gets.
		assert.Equal(t, id, req.Vars["consignmentId"])
		assert.Contains(t, req.Vars, "traderCompany")
		assert.Len(t, req.Vars, 2)
		assert.NoError(t, sqlMock.ExpectationsWereMet())
	})

	t.Run("a flow that is not listed is refused", func(t *testing.T) {
		svc, sqlMock, flows := newFlowsService(t)
		id := uuid.NewString()
		expectConsignment(sqlMock, id, "co-trader", "IN_PROGRESS")

		_, err := svc.StartConsignmentFlow(context.Background(), id, "trade-export", "co-trader", []string{"Trader"})
		assert.ErrorIs(t, err, ErrFlowNotAllowed)
		assert.Empty(t, flows.reqs)
		assert.NoError(t, sqlMock.ExpectationsWereMet())
	})

	t.Run("a flow listed for another role is refused", func(t *testing.T) {
		svc, sqlMock, flows := newFlowsService(t)
		id := uuid.NewString()
		expectConsignment(sqlMock, id, "co-trader", "IN_PROGRESS")

		_, err := svc.StartConsignmentFlow(context.Background(), id, "cha_flow", "co-trader", []string{"Trader"})
		assert.ErrorIs(t, err, ErrFlowNotAllowed)
		assert.Empty(t, flows.reqs)
	})

	t.Run("nothing starts while the list is off", func(t *testing.T) {
		svc, sqlMock, flows := newFlowsService(t)
		svc.userFlows.Enabled = false
		id := uuid.NewString()
		expectConsignment(sqlMock, id, "co-trader", "IN_PROGRESS")

		_, err := svc.StartConsignmentFlow(context.Background(), id, "aux_flow", "co-trader", []string{"Trader"})
		assert.ErrorIs(t, err, ErrFlowNotAllowed)
		assert.Empty(t, flows.reqs)
	})

	t.Run("another company is denied", func(t *testing.T) {
		svc, sqlMock, flows := newFlowsService(t)
		id := uuid.NewString()
		expectConsignment(sqlMock, id, "co-trader", "IN_PROGRESS")

		_, err := svc.StartConsignmentFlow(context.Background(), id, "aux_flow", "co-other", []string{"Trader"})
		assert.ErrorIs(t, err, ErrAccessDenied)
		assert.Empty(t, flows.reqs)
	})

	t.Run("the trader company without the trader role is denied", func(t *testing.T) {
		svc, sqlMock, flows := newFlowsService(t)
		id := uuid.NewString()
		expectConsignment(sqlMock, id, "co-trader", "IN_PROGRESS")

		_, err := svc.StartConsignmentFlow(context.Background(), id, "aux_flow", "co-trader", []string{"CHA"})
		assert.ErrorIs(t, err, ErrAccessDenied)
		assert.Empty(t, flows.reqs)
	})

	t.Run("start failure", func(t *testing.T) {
		svc, sqlMock, flows := newFlowsService(t)
		flows.err = errors.New("engine down")
		id := uuid.NewString()
		expectConsignment(sqlMock, id, "co-trader", "IN_PROGRESS")

		_, err := svc.StartConsignmentFlow(context.Background(), id, "aux_flow", "co-trader", []string{"Trader"})
		assert.ErrorContains(t, err, "engine down")
	})
}

func TestHandleStartConsignmentFlow(t *testing.T) {
	call := func(t *testing.T, svc *Service, body, consignmentID string) *httptest.ResponseRecorder {
		t.Helper()
		mockCompany := svc.companyService.(*MockCompanyService)
		mockCompany.On("GetCompanyByOUHandle", mock.Anything, "trader-ou").Return(&company.Record{ID: "co-trader", OUHandle: "trader-ou"}, nil).Maybe()
		r := mustNewRouter(t, svc, nil, mockCompany, nswaudit.New())
		req := httptest.NewRequest(http.MethodPost, "/api/v1/consignments/"+consignmentID+"/flows", strings.NewReader(body))
		req.SetPathValue("id", consignmentID)
		req = req.WithContext(withAuthContextRoles(req.Context(), "trader1", "trader-ou", "Trader"))
		w := httptest.NewRecorder()
		r.HandleStartConsignmentFlow(w, req)
		return w
	}

	t.Run("created", func(t *testing.T) {
		svc, sqlMock, _ := newFlowsService(t)
		id := uuid.NewString()
		expectConsignment(sqlMock, id, "co-trader", "IN_PROGRESS")

		w := call(t, svc, `{"templateId":"aux_flow"}`, id)
		require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
		var got FlowDTO
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
		assert.Equal(t, "aux_flow", got.TemplateID)
		assert.Equal(t, string(caseflow.StatusStarted), got.Status)
	})

	t.Run("not startable", func(t *testing.T) {
		svc, sqlMock, _ := newFlowsService(t)
		id := uuid.NewString()
		expectConsignment(sqlMock, id, "co-trader", "IN_PROGRESS")

		w := call(t, svc, `{"templateId":"trade-export"}`, id)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("denied reads as not found", func(t *testing.T) {
		svc, sqlMock, _ := newFlowsService(t)
		id := uuid.NewString()
		expectConsignment(sqlMock, id, "co-someone-else", "IN_PROGRESS")

		w := call(t, svc, `{"templateId":"aux_flow"}`, id)
		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("variables for the flow are refused", func(t *testing.T) {
		svc, _, flows := newFlowsService(t)
		w := call(t, svc, `{"templateId":"aux_flow","inputs":{"payment":{"status":"PAID"}}}`, uuid.NewString())
		assert.Equal(t, http.StatusBadRequest, w.Code)
		assert.Empty(t, flows.reqs)
	})

	t.Run("unknown field", func(t *testing.T) {
		svc, _, _ := newFlowsService(t)
		w := call(t, svc, `{"templateId":"aux_flow","workflowId":"x"}`, uuid.NewString())
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("trailing data", func(t *testing.T) {
		svc, _, flows := newFlowsService(t)
		w := call(t, svc, `{"templateId":"aux_flow"} {}`, uuid.NewString())
		assert.Equal(t, http.StatusBadRequest, w.Code)
		assert.Empty(t, flows.reqs)
	})

	t.Run("body too large", func(t *testing.T) {
		svc, _, flows := newFlowsService(t)
		body := `{"templateId":"` + strings.Repeat("a", maxStartFlowRequestBytes) + `"}`
		w := call(t, svc, body, uuid.NewString())
		assert.Equal(t, http.StatusRequestEntityTooLarge, w.Code)
		assert.Empty(t, flows.reqs)
	})
}

// The detail offers each reader the flows listed for a role they own the consignment in.
func TestDetailListsStartableFlows(t *testing.T) {
	for name, tc := range map[string]struct {
		enabled bool
		want    []FlowOption
	}{
		"the trader's flows":  {true, []FlowOption{auxFlow}},
		"none while it's off": {false, []FlowOption{}},
	} {
		t.Run(name, func(t *testing.T) {
			svc, sqlMock, _ := newFlowsService(t)
			svc.userFlows.Enabled = tc.enabled
			taskStore := new(MockTaskStore)
			svc.taskStore = taskStore
			id := uuid.NewString()
			expectConsignment(sqlMock, id, "co-trader", "IN_PROGRESS")
			taskStore.On("GetTasks", mock.Anything, store.TaskFilter{CaseID: id, Roles: []string{partyRoleTrader}}).Return(nil)

			got, err := svc.GetConsignmentByID(context.Background(), id, "co-trader", []string{"Trader"})
			require.NoError(t, err)
			assert.Equal(t, tc.want, got.StartableFlows)
		})
	}
}

// TestConsignmentDetailListsFlowsTheCallerMayStart checks the trader's permission
// and the admin view's informational list of trader and CHA flows.
func TestConsignmentDetailListsFlowsTheCallerMayStart(t *testing.T) {
	for _, tc := range []struct {
		name     string
		admin    bool
		canStart bool
		want     []FlowOption
	}{
		{name: "trader who may start flows", canStart: true, want: []FlowOption{auxFlow}},
		{name: "trader who may not start flows", want: []FlowOption{}},
		{name: "admin lists trader and CHA flows", admin: true, want: []FlowOption{auxFlow, {TemplateID: "cha_flow", Label: "CHA flow"}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc, sqlMock, _ := newFlowsService(t)
			taskStore := new(MockTaskStore)
			svc.taskStore = taskStore
			companies := new(MockCompanyService)
			id := uuid.NewString()
			expectConsignment(sqlMock, id, "co-trader", "IN_PROGRESS")
			router := mustNewRouter(t, svc, nil, companies, nil)
			router.canStartFlows = func(context.Context) bool { return tc.canStart }
			path := "/api/v1/consignments/" + id
			handler := router.HandleGetConsignmentByID
			taskFilter := store.TaskFilter{CaseID: id, Roles: []string{partyRoleTrader}}
			if tc.admin {
				path = "/api/v1/admin/consignments/" + id
				handler = router.HandleAdminGetConsignmentByID
				taskFilter.Roles = nil
			} else {
				companies.On("GetCompanyByOUHandle", mock.Anything, "trader-ou").Return(&company.Record{ID: "co-trader"}, nil)
			}
			taskStore.On("GetTasks", mock.Anything, taskFilter).Return(nil).Once()
			req := httptest.NewRequest(http.MethodGet, path, nil)
			req.SetPathValue("id", id)
			req = req.WithContext(withAuthContextRoles(req.Context(), "trader", "trader-ou", "Trader"))
			w := httptest.NewRecorder()
			handler(w, req)
			require.Equal(t, http.StatusOK, w.Code, w.Body.String())
			var got DetailDTO
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
			assert.Equal(t, tc.want, got.StartableFlows)
			taskStore.AssertExpectations(t)
		})
	}
}
