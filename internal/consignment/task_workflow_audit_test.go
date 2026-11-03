package consignment

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/OpenNSW/core/taskflow/store"
	workflow "github.com/OpenNSW/core/workflow"
	nswaudit "github.com/OpenNSW/nsw-srilanka/internal/audit"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestTaskWorkflowEngineStatus_AuditTarget(t *testing.T) {
	for _, tc := range []struct {
		name    string
		record  store.TaskRecord
		found   bool
		noStore bool
		wantID  string
	}{
		{"stored case", store.TaskRecord{CaseID: "consignment-1", ParentWorkflowID: "nested-flow"}, true, false, "consignment-1"},
		{"missing record", store.TaskRecord{}, false, false, ""},
		{"missing case", store.TaskRecord{ParentWorkflowID: "nested-flow"}, true, false, ""},
		{"no store", store.TaskRecord{}, false, true, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			const workflowID = "task-wf-review"
			tasks := new(MockTaskStore)
			var taskStore TaskStore = tasks
			if tc.noStore {
				taskStore = nil
			} else {
				tasks.On("GetTaskByWorkflowID", mock.Anything, workflowID).Return(tc.record, tc.found).Once()
			}
			svc := mustNewService(t, nil, nil, nil, nil, nil, taskStore)
			wm := new(MockWM)
			require.NoError(t, svc.RegisterTaskWorkflowManager(wm))
			// Task workflow globals can echo its own ID; they are not the audit target.
			wm.On("GetStatus", mock.Anything, workflowID).Return(&workflow.WorkflowInstance{
				ID: workflowID, WorkflowVariables: map[string]any{workflow.VarCaseID: workflowID},
			}, nil).Once()
			client, capture := nswaudit.NewWithCapture()
			router := mustNewRouter(t, svc, nil, nil, client)
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req.SetPathValue("id", workflowID)
			req = req.WithContext(withAuthContext(req.Context(), "admin-1"))
			w := httptest.NewRecorder()
			router.HandleGetTaskWorkflowEngineStatus(w, req)
			require.Equal(t, http.StatusOK, w.Code)
			assert.Contains(t, w.Body.String(), workflowID)
			records := capture.Records()
			require.Len(t, records, 1)
			assert.Equal(t, nswaudit.TargetConsignment, records[0].TargetType)
			assert.Equal(t, tc.wantID, records[0].TargetID)
			assert.Equal(t, workflowID, records[0].Metadata["workflowId"])
			assert.Equal(t, "task-workflow-engine-status", records[0].Metadata["view"])
			assert.Equal(t, nswaudit.StatusSuccess, records[0].Status)
			if tc.wantID == "" {
				assert.NotEmpty(t, records[0].Metadata["targetResolutionError"])
			}
			tasks.AssertExpectations(t)
			wm.AssertExpectations(t)
		})
	}
}

func TestTaskWorkflowActions_AuditTarget(t *testing.T) {
	for _, forcePark := range []bool{false, true} {
		for _, fail := range []bool{false, true} {
			name := "resolve"
			if forcePark {
				name = "force-park"
			}
			if fail {
				name += "/failure"
			} else {
				name += "/success"
			}
			t.Run(name, func(t *testing.T) {
				const workflowID = "task-wf-review"
				tasks := new(MockTaskStore)
				tasks.On("GetTaskByWorkflowID", mock.Anything, workflowID).Return(store.TaskRecord{CaseID: "consignment-1"}, true).Once()
				svc := mustNewService(t, nil, nil, nil, nil, nil, tasks)
				wm := new(MockWM)
				require.NoError(t, svc.RegisterTaskWorkflowManager(wm))
				instance := parkedInstance(workflow.NodeTypeTask)
				if forcePark {
					instance.NodeInfo[resolveTestNodeID].Status = workflow.NodeStatusRunning
				}
				if fail {
					instance.NodeInfo = nil
				}
				wm.On("GetStatus", mock.Anything, workflowID).Return(instance, nil).Once()
				if !fail {
					if forcePark {
						wm.On("ForceParkActivation", mock.Anything, workflowID, "", resolveTestStepID, "repair").Return(nil).Once()
					} else {
						wm.On("ResolveAdminIntervention", mock.Anything, workflowID, "", mock.Anything).Return(nil).Once()
					}
				}
				client, capture := nswaudit.NewWithCapture()
				router := mustNewRouter(t, svc, nil, nil, client)
				body := `{"action":"RETRY","reason":"repair"}`
				if forcePark {
					body = `{"reason":"repair"}`
				}
				req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
				req.SetPathValue("id", workflowID)
				req.SetPathValue("stepId", resolveTestStepID)
				req = req.WithContext(withAuthContext(req.Context(), "admin-1"))
				w := httptest.NewRecorder()
				if forcePark {
					router.HandleForceParkTaskWorkflowStep(w, req)
				} else {
					router.HandleResolveTaskWorkflowAdminIntervention(w, req)
				}
				wantCode, wantStatus := http.StatusNoContent, nswaudit.StatusSuccess
				if fail {
					wantCode, wantStatus = http.StatusConflict, nswaudit.StatusFailure
				}
				require.Equal(t, wantCode, w.Code)
				records := capture.Records()
				require.Len(t, records, 1)
				assert.Equal(t, "consignment-1", records[0].TargetID)
				assert.Equal(t, workflowID, records[0].Metadata["workflowId"])
				assert.Equal(t, resolveTestStepID, records[0].Metadata["stepId"])
				assert.Equal(t, wantStatus, records[0].Status)
				tasks.AssertExpectations(t)
				wm.AssertExpectations(t)
			})
		}
	}
}

func TestWorkflowEngineStatus_AuditTargetUnchanged(t *testing.T) {
	tasks := new(MockTaskStore) // A flow audit must not use the task-workflow lookup.
	tasks.On("GetAllTasks", mock.Anything, "flow-1").Return([]store.TaskRecord(nil)).Once()
	svc := mustNewService(t, nil, nil, nil, nil, nil, tasks)
	wm := new(MockWM)
	require.NoError(t, svc.RegisterWorkflowManager(wm))
	wm.On("GetStatus", mock.Anything, "flow-1").Return(&workflow.WorkflowInstance{ID: "flow-1", WorkflowVariables: map[string]any{workflow.VarCaseID: "flow-1"}}, nil).Once()
	client, capture := nswaudit.NewWithCapture()
	router := mustNewRouter(t, svc, nil, nil, client)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.SetPathValue("id", "flow-1")
	req = req.WithContext(withAuthContext(req.Context(), "admin-1"))
	w := httptest.NewRecorder()
	router.HandleGetWorkflowEngineStatus(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	records := capture.Records()
	require.Len(t, records, 1)
	assert.Equal(t, "flow-1", records[0].TargetID)
	assert.Equal(t, "engine-status", records[0].Metadata["view"])
	assert.Equal(t, "flow-1", records[0].Metadata["workflowId"])
	wm.AssertExpectations(t)
	tasks.AssertExpectations(t)
	tasks.AssertNotCalled(t, "GetTaskByWorkflowID", mock.Anything, mock.Anything)
}
