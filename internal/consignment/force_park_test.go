package consignment

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/OpenNSW/core/taskflow/orchestrator"
	workflow "github.com/OpenNSW/core/workflow"
	nswaudit "github.com/OpenNSW/nsw-srilanka/internal/audit"
)

// fakeTaskCanceller records the tasks it is asked to cancel, in calls shared with the test's
// workflow manager so their order can be checked.
type fakeTaskCanceller struct {
	calls *[]string
	err   error
}

func (f *fakeTaskCanceller) CancelTask(_ context.Context, taskID, reason string) error {
	*f.calls = append(*f.calls, "cancel "+taskID+" ("+reason+")")
	return f.err
}

// waitingInstance is a workflow with one node of nodeType running under resolveTestStepID, as a
// TASK node is while it waits on its step.
func waitingInstance(nodeType workflow.NodeType) *workflow.WorkflowInstance {
	inst := parkedInstance(nodeType)
	inst.NodeInfo[resolveTestNodeID].Status = workflow.NodeStatusRunning
	return inst
}

// forceParkRoute is one of the two force-park endpoints, with the service method behind it.
type forceParkRoute struct {
	name    string
	own     managerSlot
	other   managerSlot
	park    func(s *Service) func(ctx context.Context, workflowID, stepID, reason string) (string, error)
	handler func(r *Router) http.HandlerFunc
	// cancels says whether the route closes the node's task before parking it.
	cancels bool
}

var forceParkRoutes = []forceParkRoute{
	{
		name: "consignment route", own: rootManager, other: taskManager, cancels: true,
		park: func(s *Service) func(context.Context, string, string, string) (string, error) {
			return s.ForceParkTaskNode
		},
		handler: func(r *Router) http.HandlerFunc { return r.HandleForceParkTaskNode },
	},
	{
		name: "task workflow route", own: taskManager, other: rootManager, cancels: false,
		park: func(s *Service) func(context.Context, string, string, string) (string, error) {
			return s.ForceParkTaskWorkflowStep
		},
		handler: func(r *Router) http.HandlerFunc { return r.HandleForceParkTaskWorkflowStep },
	},
}

// forceParkFixture builds a Service with mockWM as the route's own manager, an unused manager in the
// other slot, and a canceller returning cancelErr. Both record into the returned calls.
func forceParkFixture(t *testing.T, route forceParkRoute, cancelErr error) (*Service, *MockWM, *[]string) {
	t.Helper()
	calls := &[]string{}
	mockWM, otherWM := new(MockWM), new(MockWM)
	otherWM.Test(t) // any call on the other manager fails the test
	db, _ := setupTestDB(t)
	svc := mustNewService(t, db, nil, nil, nil, nil, nil)
	require.NoError(t, route.own(svc, mockWM))
	require.NoError(t, route.other(svc, otherWM))
	require.NoError(t, svc.RegisterTaskCanceller(&fakeTaskCanceller{calls: calls, err: cancelErr}))
	return svc, mockWM, calls
}

// expectPark sets up the step's park on mockWM, recording it into calls, and returning err.
func expectPark(mockWM *MockWM, calls *[]string, err error) {
	mockWM.On("ForceParkActivation", mock.Anything, resolveTestWorkflowID, "", resolveTestStepID, "warranted offline").
		Run(func(mock.Arguments) { *calls = append(*calls, "park") }).
		Return(err)
}

// The consignment route parks the node first (so it is visibly parked and stops accepting the
// task's completion), then closes the node's task. The task workflow route parks the step alone and
// leaves its task open.
func TestForcePark_ParksTheStep(t *testing.T) {
	for _, route := range forceParkRoutes {
		t.Run(route.name, func(t *testing.T) {
			svc, mockWM, calls := forceParkFixture(t, route, nil)
			mockWM.On("GetStatus", mock.Anything, resolveTestWorkflowID).Return(waitingInstance(workflow.NodeTypeTask), nil)
			expectPark(mockWM, calls, nil)

			nodeID, err := route.park(svc)(context.Background(), resolveTestWorkflowID, resolveTestStepID, "warranted offline")

			require.NoError(t, err)
			assert.Equal(t, resolveTestNodeID, nodeID)
			want := []string{"park"}
			if route.cancels {
				want = []string{"park", "cancel " + resolveTestStepID + " (warranted offline)"}
			}
			assert.Equal(t, want, *calls)
			mockWM.AssertExpectations(t)
		})
	}
}

func TestForcePark_RejectsNodesNotWaiting(t *testing.T) {
	tests := []struct {
		name     string
		instance *workflow.WorkflowInstance
		wantErr  error
	}{
		{"an unknown step", func() *workflow.WorkflowInstance {
			inst := waitingInstance(workflow.NodeTypeTask)
			inst.NodeInfo[resolveTestNodeID].ActivationID = "step-2"
			return inst
		}(), ErrNodeNotWaiting},
		{"a node already parked", parkedInstance(workflow.NodeTypeTask), ErrNodeNotWaiting},
		{"a completed node", func() *workflow.WorkflowInstance {
			inst := waitingInstance(workflow.NodeTypeTask)
			inst.NodeInfo[resolveTestNodeID].Status = workflow.NodeStatusCompleted
			return inst
		}(), ErrNodeNotWaiting},
		{"a split node", waitingInstance(workflow.NodeTypeSplitTask), ErrForceParkUnsupportedNodeType},
		{"a gateway", waitingInstance(workflow.NodeTypeGateway), ErrForceParkUnsupportedNodeType},
	}
	for _, route := range forceParkRoutes {
		for _, tt := range tests {
			t.Run(route.name+"/"+tt.name, func(t *testing.T) {
				svc, mockWM, calls := forceParkFixture(t, route, nil)
				mockWM.On("GetStatus", mock.Anything, resolveTestWorkflowID).Return(tt.instance, nil)

				_, err := route.park(svc)(context.Background(), resolveTestWorkflowID, resolveTestStepID, "warranted offline")

				assert.ErrorIs(t, err, tt.wantErr)
				assert.Empty(t, *calls)
			})
		}
	}
}

// If the task finished in the gap right after ForceParkActivation parked the parent node, its
// completion was rejected by the parent workflow and the task is already closed: ForceParkTaskNode
// still succeeds.
func TestForceParkTaskNode_TaskClosedInTheGapAfterParkSucceeds(t *testing.T) {
	svc, mockWM, calls := forceParkFixture(t, forceParkRoutes[0], fmt.Errorf("%w: task has completed", orchestrator.ErrTaskNotOpen))
	mockWM.On("GetStatus", mock.Anything, resolveTestWorkflowID).Return(waitingInstance(workflow.NodeTypeTask), nil)
	expectPark(mockWM, calls, nil)

	nodeID, err := svc.ForceParkTaskNode(context.Background(), resolveTestWorkflowID, resolveTestStepID, "warranted offline")

	require.NoError(t, err)
	assert.Equal(t, resolveTestNodeID, nodeID)
	assert.Equal(t, []string{"park", "cancel " + resolveTestStepID + " (warranted offline)"}, *calls)
	mockWM.AssertExpectations(t)
}

// If ForceParkActivation succeeds and CancelTask then fails, the node is visibly parked with
// ParkCategoryAdminForced, and repeating ForceParkTaskNode finishes cancelling the task without
// calling ForceParkActivation again.
func TestForceParkTaskNode_CancelFailureCanBeRetriedOnceParked(t *testing.T) {
	svc, mockWM, calls := forceParkFixture(t, forceParkRoutes[0], errors.New("temporal unavailable"))
	mockWM.On("GetStatus", mock.Anything, resolveTestWorkflowID).Return(waitingInstance(workflow.NodeTypeTask), nil).Once()
	expectPark(mockWM, calls, nil)

	_, err := svc.ForceParkTaskNode(context.Background(), resolveTestWorkflowID, resolveTestStepID, "warranted offline")
	require.Error(t, err)
	assert.NotErrorIs(t, err, ErrNodeNotWaiting)
	assert.Equal(t, []string{"park", "cancel " + resolveTestStepID + " (warranted offline)"}, *calls)

	// Retry: the node is now AWAITING_ADMIN with ParkCategoryAdminForced.
	alreadyParked := parkedInstance(workflow.NodeTypeTask)
	alreadyParked.NodeInfo[resolveTestNodeID].ParkCategory = workflow.ParkCategoryAdminForced
	mockWM.On("GetStatus", mock.Anything, resolveTestWorkflowID).Return(alreadyParked, nil).Once()
	svc.tasks.(*fakeTaskCanceller).err = nil

	nodeID, err := svc.ForceParkTaskNode(context.Background(), resolveTestWorkflowID, resolveTestStepID, "warranted offline")
	require.NoError(t, err)
	assert.Equal(t, resolveTestNodeID, nodeID)
	assert.Equal(t, []string{
		"park",
		"cancel " + resolveTestStepID + " (warranted offline)",
		"cancel " + resolveTestStepID + " (warranted offline)",
	}, *calls)
	mockWM.AssertExpectations(t)
}

// The step finished while the request was in flight.
func TestForcePark_StepNoLongerPending(t *testing.T) {
	for _, route := range forceParkRoutes {
		t.Run(route.name, func(t *testing.T) {
			svc, mockWM, calls := forceParkFixture(t, route, nil)
			mockWM.On("GetStatus", mock.Anything, resolveTestWorkflowID).Return(waitingInstance(workflow.NodeTypeTask), nil)
			expectPark(mockWM, calls, fmt.Errorf("%w: gone", workflow.ErrActivationNotPending))

			_, err := route.park(svc)(context.Background(), resolveTestWorkflowID, resolveTestStepID, "warranted offline")

			assert.ErrorIs(t, err, ErrNodeNotWaiting)
		})
	}
}

func TestForcePark_WorkflowNotFound(t *testing.T) {
	for _, route := range forceParkRoutes {
		t.Run(route.name, func(t *testing.T) {
			svc, mockWM, _ := forceParkFixture(t, route, nil)
			mockWM.On("GetStatus", mock.Anything, resolveTestWorkflowID).
				Return((*workflow.WorkflowInstance)(nil), fmt.Errorf("%w: gone", workflow.ErrWorkflowNotFound))

			_, err := route.park(svc)(context.Background(), resolveTestWorkflowID, resolveTestStepID, "warranted offline")

			assert.ErrorIs(t, err, ErrEngineWorkflowNotFound)
		})
	}
}

func TestForceParkTaskNode_NoTaskCanceller(t *testing.T) {
	db, _ := setupTestDB(t)
	svc := mustNewService(t, db, nil, nil, nil, nil, nil)
	require.NoError(t, svc.RegisterWorkflowManager(new(MockWM)))

	_, err := svc.ForceParkTaskNode(context.Background(), resolveTestWorkflowID, resolveTestStepID, "reason")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "no task canceller")
}

func newForceParkRequest(body string) *http.Request {
	req := newResolveRequest(body)
	req.URL.Path = strings.TrimSuffix(req.URL.Path, "/resolve") + "/force-park"
	return req
}

func TestRouter_HandleForcePark_MapsResults(t *testing.T) {
	tests := []struct {
		name       string
		instance   *workflow.WorkflowInstance
		statusErr  error
		parkErr    error
		wantStatus int
	}{
		{"parked is 204", waitingInstance(workflow.NodeTypeTask), nil, nil, http.StatusNoContent},
		{"workflow not found is 404", nil, fmt.Errorf("%w: gone", workflow.ErrWorkflowNotFound), nil, http.StatusNotFound},
		{"node not waiting is 409", parkedInstance(workflow.NodeTypeTask), nil, nil, http.StatusConflict},
		{"step no longer pending is 409", waitingInstance(workflow.NodeTypeTask), nil, workflow.ErrActivationNotPending, http.StatusConflict},
		{"a non-TASK node is 400", waitingInstance(workflow.NodeTypeGateway), nil, nil, http.StatusBadRequest},
		{"a Temporal failure is 500", waitingInstance(workflow.NodeTypeTask), nil, errors.New("temporal unavailable"), http.StatusInternalServerError},
	}
	for _, route := range forceParkRoutes {
		for _, tt := range tests {
			t.Run(route.name+"/"+tt.name, func(t *testing.T) {
				svc, mockWM, calls := forceParkFixture(t, route, nil)
				client, _ := nswaudit.NewWithCapture()
				r := mustNewRouter(t, svc, nil, nil, client)
				mockWM.On("GetStatus", mock.Anything, resolveTestWorkflowID).Return(tt.instance, tt.statusErr)
				expectPark(mockWM, calls, tt.parkErr)

				w := httptest.NewRecorder()
				route.handler(r)(w, newForceParkRequest(`{"reason":"warranted offline"}`))

				assert.Equal(t, tt.wantStatus, w.Code)
			})
		}
	}
}

// Requests the handler itself rejects never reach the service, so nothing is registered.
func TestRouter_HandleForcePark_RejectsInvalidRequest(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		stepID  string
		wantErr string
	}{
		{"a missing reason", `{}`, resolveTestStepID, errReasonRequired},
		{"a blank reason", `{"reason":"  "}`, resolveTestStepID, errReasonRequired},
		{"a body that is not JSON", `not json`, resolveTestStepID, errInvalidRequestBody},
		{"anything after the JSON value", `{"reason":"a"}{"reason":"b"}`, resolveTestStepID, errInvalidRequestBody},
		{"an unknown field", `{"reason":"a","action":"COMPLETE"}`, resolveTestStepID, `unknown field \"action\"`},
		{"a missing step", `{"reason":"a"}`, "", errStepIDRequired},
	}
	for _, route := range forceParkRoutes {
		for _, tt := range tests {
			t.Run(route.name+"/"+tt.name, func(t *testing.T) {
				client, _ := nswaudit.NewWithCapture()
				r := mustNewRouter(t, mustNewService(t, nil, nil, nil, nil, nil, nil), nil, nil, client)
				req := newForceParkRequest(tt.body)
				req.SetPathValue("stepId", tt.stepID)

				w := httptest.NewRecorder()
				route.handler(r)(w, req)

				assert.Equal(t, http.StatusBadRequest, w.Code)
				assert.Contains(t, w.Body.String(), tt.wantErr)
			})
		}
	}
}
