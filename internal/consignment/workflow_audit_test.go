package consignment

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	workflow "github.com/OpenNSW/core/workflow"
	nswaudit "github.com/OpenNSW/nsw-srilanka/internal/audit"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestWorkflowEngineStatus_AuditCaseID(t *testing.T) {
	for _, tc := range []struct {
		name      string
		variables map[string]any
		wantID    string
	}{
		{"auxiliary flow", map[string]any{workflow.VarCaseID: "consignment-1"}, "consignment-1"},
		{"missing case", nil, ""},
		{"invalid case", map[string]any{workflow.VarCaseID: 42}, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc := mustNewService(t, nil, nil, nil, nil, nil, nil)
			wm := new(MockWM)
			require.NoError(t, svc.RegisterWorkflowManager(wm))
			wm.On("GetStatus", mock.Anything, "auxiliary--child").Return(&workflow.WorkflowInstance{
				ID: "auxiliary--child", WorkflowVariables: tc.variables,
			}, nil).Once()
			client, capture := nswaudit.NewWithCapture()
			router := mustNewRouter(t, svc, nil, nil, client)
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req.SetPathValue("id", "auxiliary--child")
			req = req.WithContext(withAuthContext(req.Context(), "admin-1"))
			w := httptest.NewRecorder()
			router.HandleGetWorkflowEngineStatus(w, req)
			require.Equal(t, http.StatusOK, w.Code)
			assert.Contains(t, w.Body.String(), `"workflow_id":"auxiliary--child"`)
			records := capture.Records()
			require.Len(t, records, 1)
			assert.Equal(t, tc.wantID, records[0].TargetID)
			assert.Equal(t, nswaudit.TargetConsignment, records[0].TargetType)
			assert.Equal(t, "auxiliary--child", records[0].Metadata["workflowId"])
			assert.Equal(t, "engine-status", records[0].Metadata["view"])
			assert.Equal(t, nswaudit.StatusSuccess, records[0].Status)
			if tc.wantID == "" {
				assert.NotEmpty(t, records[0].Metadata["targetResolutionError"])
			}
			wm.AssertExpectations(t)
		})
	}
}

func TestWorkflowActions_AuditCaseID(t *testing.T) {
	for _, route := range []struct {
		name      string
		handler   func(*Router) http.HandlerFunc
		forcePark bool
	}{
		{"resolve", func(r *Router) http.HandlerFunc { return r.HandleResolveAdminIntervention }, false},
		{"force-park", func(r *Router) http.HandlerFunc { return r.HandleForceParkTaskNode }, true},
	} {
		for _, tc := range []struct {
			name        string
			fail        bool
			lookupFails bool
		}{
			{"success", false, false}, {"conflict", true, false}, {"lookup failure", false, true},
		} {
			t.Run(route.name+"/"+tc.name, func(t *testing.T) {
				svc := mustNewService(t, nil, nil, nil, nil, nil, nil)
				wm := new(MockWM)
				require.NoError(t, svc.RegisterWorkflowManager(wm))
				instance := parkedInstance(workflow.NodeTypeTask)
				instance.WorkflowVariables = map[string]any{workflow.VarCaseID: "consignment-1"}
				if route.forcePark {
					instance.NodeInfo[resolveTestNodeID].Status = workflow.NodeStatusRunning
					calls := []string{}
					canceller := &fakeTaskCanceller{calls: &calls}
					require.NoError(t, svc.RegisterTaskCanceller(canceller))

				}
				if tc.fail {
					instance.NodeInfo = nil
				}
				wm.On("GetStatus", mock.Anything, resolveTestWorkflowID).Return(instance, nil).Once()
				if tc.lookupFails {
					wm.On("GetStatus", mock.Anything, resolveTestWorkflowID).Return((*workflow.WorkflowInstance)(nil), errors.New("temporal unavailable")).Once()
				} else {
					wm.On("GetStatus", mock.Anything, resolveTestWorkflowID).Return(instance, nil).Once()
				}
				if !tc.fail {
					if route.forcePark {
						wm.On("ForceParkActivation", mock.Anything, resolveTestWorkflowID, "", resolveTestStepID, "repair").Return(nil).Once()
					} else {
						wm.On("ResolveAdminIntervention", mock.Anything, resolveTestWorkflowID, "", mock.Anything).Return(nil).Once()
					}
				}
				client, capture := nswaudit.NewWithCapture()
				router := mustNewRouter(t, svc, nil, nil, client)
				body := `{"action":"RETRY","reason":"repair"}`
				if route.forcePark {
					body = `{"reason":"repair"}`
				}
				req := newResolveRequest(body)
				w := httptest.NewRecorder()
				route.handler(router)(w, req)
				wantCode, wantStatus := http.StatusNoContent, nswaudit.StatusSuccess
				if tc.fail {
					wantCode, wantStatus = http.StatusConflict, nswaudit.StatusFailure
				}
				require.Equal(t, wantCode, w.Code)
				records := capture.Records()
				require.Len(t, records, 1)
				wantID := "consignment-1"
				if tc.lookupFails {
					wantID = ""
					assert.NotEmpty(t, records[0].Metadata["targetResolutionError"])
				}
				assert.Equal(t, wantID, records[0].TargetID)
				assert.Equal(t, resolveTestWorkflowID, records[0].Metadata["workflowId"])
				assert.Equal(t, resolveTestStepID, records[0].Metadata["stepId"])
				assert.Equal(t, wantStatus, records[0].Status)
				wm.AssertExpectations(t)
			})
		}
	}
}

func TestTaskWorkflowEngineStatus_DoesNotUseFlowAuditLookup(t *testing.T) {
	svc := mustNewService(t, nil, nil, nil, nil, nil, nil)
	rootWM, taskWM := new(MockWM), new(MockWM)
	require.NoError(t, svc.RegisterWorkflowManager(rootWM))
	require.NoError(t, svc.RegisterTaskWorkflowManager(taskWM))
	taskWM.On("GetStatus", mock.Anything, "task-wf-1").Return(&workflow.WorkflowInstance{ID: "task-wf-1"}, nil).Once()
	client, capture := nswaudit.NewWithCapture()
	router := mustNewRouter(t, svc, nil, nil, client)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.SetPathValue("id", "task-wf-1")
	req = req.WithContext(withAuthContext(req.Context(), "admin-1"))
	w := httptest.NewRecorder()
	router.HandleGetTaskWorkflowEngineStatus(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	records := capture.Records()
	require.Len(t, records, 1)
	assert.Equal(t, nswaudit.TargetConsignment, records[0].TargetType)
	assert.Equal(t, "task-workflow-engine-status", records[0].Metadata["view"])
	rootWM.AssertNotCalled(t, "GetStatus", mock.Anything, mock.Anything)
	taskWM.AssertExpectations(t)
}
