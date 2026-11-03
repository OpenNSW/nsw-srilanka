package workflowadmin

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	workflow "github.com/OpenNSW/core/workflow"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

const (
	resolveTestWorkflowID = "wf-1"
	resolveTestNodeID     = "officer_review"
	// resolveTestStepID is the parked node's current step ID, which every request addresses.
	resolveTestStepID = "step-1"
)

// managerSlot registers a mock as one of the service's two workflow managers: the root one, for a
// consignment's root and child-branch workflows, or the task one, for task workflows.
type managerSlot func(svc *Service, m *MockWM) error

var (
	rootManager managerSlot = func(svc *Service, m *MockWM) error { return svc.RegisterWorkflowManager(m) }
	taskManager managerSlot = func(svc *Service, m *MockWM) error { return svc.RegisterTaskWorkflowManager(m) }
)

// resolveRoute is one of the two resolve endpoints. They share all their request handling and
// differ only in which of the service's managers they use, so most tests below run against both.
type resolveRoute struct {
	name    string
	own     managerSlot // the manager this route must use
	other   managerSlot // the manager it must never touch
	handler func(r *Handler) http.HandlerFunc
}

var resolveRoutes = []resolveRoute{
	{"consignment route", rootManager, taskManager, func(r *Handler) http.HandlerFunc { return r.HandleResolveAdminIntervention }},
	{"task workflow route", taskManager, rootManager, func(r *Handler) http.HandlerFunc { return r.HandleResolveTaskWorkflowAdminIntervention }},
}

// newResolveRequest builds a POST to a resolve endpoint as an authenticated admin, addressing
// resolveTestStepID on resolveTestWorkflowID. The handlers read their path values, not the
// URL, so the same request serves both routes.
func newResolveRequest(body string) *http.Request {
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/workflows/"+resolveTestWorkflowID+"/steps/"+resolveTestStepID+"/resolve", strings.NewReader(body))
	req.SetPathValue("id", resolveTestWorkflowID)
	req.SetPathValue("stepId", resolveTestStepID)
	return req.WithContext(withAuthContext(req.Context(), "admin-1"))
}

// newResolveRouter builds a Router whose service has mockWM registered as the manager route uses.
func newResolveRouter(t *testing.T, route resolveRoute, mockWM *MockWM) *Handler {
	t.Helper()
	svc := newTestService(nil)
	require.NoError(t, route.own(svc, mockWM))
	return NewHandler(svc, nil, nil)
}

// parkedInstance is a workflow with one node of nodeType parked in AWAITING_ADMIN under
// resolveTestStepID, like a real engine status.
func parkedInstance(nodeType workflow.NodeType) *workflow.WorkflowInstance {
	return &workflow.WorkflowInstance{
		ID:     resolveTestWorkflowID,
		Status: workflow.StatusRunning,
		NodeInfo: map[string]*workflow.NodeInfo{
			resolveTestNodeID: {ID: resolveTestNodeID, ActivationID: resolveTestStepID, Type: nodeType, Status: workflow.NodeStatusAwaitingAdmin},
		},
	}
}

// The request's fields must reach core's signal under core's names: the wire field
// global_variables_patch becomes WorkflowVariablesPatch, the path's step becomes ActivationID
// (core's routing key), and NodeID is the node found parked under it. A slip in this mapping would drop the admin's variables while still
// returning 204, so assert the whole signal rather than just that one was sent.
func TestHandler_HandleResolveAdminIntervention_ForwardsRequestToSignal(t *testing.T) {
	for _, route := range resolveRoutes {
		t.Run(route.name, func(t *testing.T) {
			mockWM := new(MockWM)
			r := newResolveRouter(t, route, mockWM)
			mockWM.On("GetStatus", mock.Anything, resolveTestWorkflowID).Return(parkedInstance(workflow.NodeTypeTask), nil)
			var got workflow.AdminResolutionSignal
			mockWM.On("ResolveAdminIntervention", mock.Anything, resolveTestWorkflowID, "", mock.Anything).
				Run(func(args mock.Arguments) { got = args.Get(3).(workflow.AdminResolutionSignal) }).
				Return(nil)

			body := `{"action":"COMPLETE","global_variables_patch":{"review.outcome":"APPROVED"},"reason":"result was recorded under the wrong key"}`
			w := httptest.NewRecorder()
			route.handler(r)(w, newResolveRequest(body))

			assert.Equal(t, http.StatusNoContent, w.Code)
			assert.Equal(t, workflow.AdminResolutionSignal{
				NodeID:                 resolveTestNodeID,
				ActivationID:           resolveTestStepID,
				Action:                 workflow.AdminActionComplete,
				WorkflowVariablesPatch: map[string]any{"review.outcome": "APPROVED"},
				Reason:                 "result was recorded under the wrong key",
			}, got)
			mockWM.AssertExpectations(t)
		})
	}
}

// Each route resolves through its own manager and never the other's. Both are registered here, and
// only the route's own one has expectations, so a call to the other one fails the test.
func TestHandler_HandleResolveAdminIntervention_UsesOnlyItsOwnManager(t *testing.T) {
	for _, route := range resolveRoutes {
		t.Run(route.name, func(t *testing.T) {
			ownWM, otherWM := new(MockWM), new(MockWM)
			otherWM.Test(t) // an unexpected call fails this test cleanly instead of panicking
			svc := newTestService(nil)
			require.NoError(t, route.own(svc, ownWM))
			require.NoError(t, route.other(svc, otherWM))
			r := NewHandler(svc, nil, nil)
			ownWM.On("GetStatus", mock.Anything, resolveTestWorkflowID).Return(parkedInstance(workflow.NodeTypeTask), nil)
			ownWM.On("ResolveAdminIntervention", mock.Anything, resolveTestWorkflowID, "", mock.Anything).Return(nil)

			w := httptest.NewRecorder()
			route.handler(r)(w, newResolveRequest(`{"action":"RETRY","reason":"retry"}`))

			assert.Equal(t, http.StatusNoContent, w.Code)
			ownWM.AssertExpectations(t)
			otherWM.AssertNotCalled(t, "GetStatus", mock.Anything, mock.Anything)
			otherWM.AssertNotCalled(t, "ResolveAdminIntervention", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
		})
	}
}

// A route whose own manager isn't registered fails, rather than quietly resolving through the
// other one: the two ID spaces are not interchangeable as far as the service is concerned.
func TestHandler_HandleResolveAdminIntervention_DoesNotFallBackToTheOtherManager(t *testing.T) {
	for _, route := range resolveRoutes {
		t.Run(route.name, func(t *testing.T) {
			otherWM := new(MockWM)
			otherWM.Test(t) // an unexpected call fails this test cleanly instead of panicking
			svc := newTestService(nil)
			require.NoError(t, route.other(svc, otherWM))
			r := NewHandler(svc, nil, nil)

			w := httptest.NewRecorder()
			route.handler(r)(w, newResolveRequest(`{"action":"RETRY","reason":"retry"}`))

			assert.Equal(t, http.StatusInternalServerError, w.Code)
			otherWM.AssertNotCalled(t, "GetStatus", mock.Anything, mock.Anything)
		})
	}
}

func TestHandler_HandleResolveAdminIntervention_MapsServiceErrors(t *testing.T) {
	tests := []struct {
		name       string
		body       string
		setup      func(m *MockWM)
		wantStatus int
	}{
		{
			name: "workflow not found is 404",
			body: `{"action":"RETRY","reason":"retry"}`,
			setup: func(m *MockWM) {
				m.On("GetStatus", mock.Anything, resolveTestWorkflowID).
					Return((*workflow.WorkflowInstance)(nil), fmt.Errorf("%w: gone", workflow.ErrWorkflowNotFound))
			},
			wantStatus: http.StatusNotFound,
		},
		{
			name: "node no longer parked is 409",
			body: `{"action":"RETRY","reason":"retry"}`,
			setup: func(m *MockWM) {
				inst := parkedInstance(workflow.NodeTypeTask)
				inst.NodeInfo["officer_review"].Status = workflow.NodeStatusRunning
				m.On("GetStatus", mock.Anything, resolveTestWorkflowID).Return(inst, nil)
			},
			wantStatus: http.StatusConflict,
		},
		{
			name: "stale step ID is 409",
			body: `{"action":"RETRY","reason":"retry"}`,
			setup: func(m *MockWM) {
				// Resolved and parked again since the admin loaded the engine status.
				inst := parkedInstance(workflow.NodeTypeTask)
				inst.NodeInfo[resolveTestNodeID].ActivationID = "step-2"
				m.On("GetStatus", mock.Anything, resolveTestWorkflowID).Return(inst, nil)
			},
			wantStatus: http.StatusConflict,
		},
		{
			name: "COMPLETE on a gateway is 400",
			body: `{"action":"COMPLETE","reason":"complete"}`,
			setup: func(m *MockWM) {
				m.On("GetStatus", mock.Anything, resolveTestWorkflowID).Return(parkedInstance(workflow.NodeTypeGateway), nil)
			},
			wantStatus: http.StatusBadRequest,
		},
		{
			name: "signal failure is 500",
			body: `{"action":"RETRY","reason":"retry"}`,
			setup: func(m *MockWM) {
				m.On("GetStatus", mock.Anything, resolveTestWorkflowID).Return(parkedInstance(workflow.NodeTypeTask), nil)
				m.On("ResolveAdminIntervention", mock.Anything, resolveTestWorkflowID, "", mock.Anything).Return(errors.New("temporal unavailable"))
			},
			wantStatus: http.StatusInternalServerError,
		},
	}
	for _, route := range resolveRoutes {
		for _, tt := range tests {
			t.Run(route.name+"/"+tt.name, func(t *testing.T) {
				mockWM := new(MockWM)
				r := newResolveRouter(t, route, mockWM)
				tt.setup(mockWM)

				w := httptest.NewRecorder()
				route.handler(r)(w, newResolveRequest(tt.body))

				assert.Equal(t, tt.wantStatus, w.Code)
				mockWM.AssertExpectations(t)
			})
		}
	}
}

// Requests the handler itself rejects never reach the workflow manager, so none is registered.
func TestHandler_HandleResolveAdminIntervention_RejectsInvalidRequest(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		wantErr string
	}{
		{"a removed action", `{"action":"OVERRIDE","reason":"old client"}`, errInvalidAdminAction},
		{"an unknown action", `{"action":"EXPLODE","reason":"nope"}`, errInvalidAdminAction},
		{"a lower-case action", `{"action":"retry","reason":"nope"}`, errInvalidAdminAction},
		{"a missing reason", `{"action":"RETRY"}`, errReasonRequired},
		{"a blank reason", `{"action":"RETRY","reason":"   "}`, errReasonRequired},
		{"a body that is not JSON", `not json`, errInvalidRequestBody},
		{"an empty body", ``, errInvalidRequestBody},
		{"anything after the JSON value", `{"action":"RETRY","reason":"first"}{"action":"ABORT","reason":"second"}`, errInvalidRequestBody},
		// A patch sent under a field the endpoint doesn't define (here the pre-rename
		// "overrides") must be rejected, not dropped while the action goes ahead with an empty patch.
		{"an unknown field", `{"action":"COMPLETE","overrides":{"review.outcome":"APPROVED"},"reason":"stale client"}`, `unknown field \"overrides\"`},
	}
	for _, route := range resolveRoutes {
		for _, tt := range tests {
			t.Run(route.name+"/"+tt.name, func(t *testing.T) {
				r := NewHandler(newTestService(nil), nil, nil)

				w := httptest.NewRecorder()
				route.handler(r)(w, newResolveRequest(tt.body))

				assert.Equal(t, http.StatusBadRequest, w.Code)
				assert.Contains(t, w.Body.String(), tt.wantErr)
			})
		}
	}
}

// A request without a step never reaches the workflow manager, so none is registered.
func TestHandler_HandleResolveAdminIntervention_RequiresStepID(t *testing.T) {
	for _, route := range resolveRoutes {
		t.Run(route.name, func(t *testing.T) {
			r := NewHandler(newTestService(nil), nil, nil)
			req := newResolveRequest(`{"action":"RETRY","reason":"retry"}`)
			req.SetPathValue("stepId", "")

			w := httptest.NewRecorder()
			route.handler(r)(w, req)

			assert.Equal(t, http.StatusBadRequest, w.Code)
			assert.Contains(t, w.Body.String(), errStepIDRequired)
		})
	}
}
