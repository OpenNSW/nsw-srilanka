package consignment

import (
	"context"
	"testing"
	"time"

	"fmt"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/OpenNSW/core/taskflow/store"
	workflow "github.com/OpenNSW/core/workflow"
)

func TestConsignmentService_GetEngineStatus(t *testing.T) {
	db, _ := setupTestDB(t)
	mockWM := new(MockWM)
	svc := mustNewService(t, db, nil, nil, nil, nil, nil)
	require.NoError(t, svc.RegisterWorkflowManager(mockWM))

	ctx := context.Background()
	consignmentID := uuid.NewString()
	now := time.Now()

	instance := &workflow.WorkflowInstance{
		ID:                consignmentID,
		Status:            workflow.StatusRunning,
		WorkflowVariables: map[string]any{"declared_value": float64(1000)},
		NodeInfo: map[string]*workflow.NodeInfo{
			"node-b": {ID: "node-b", Type: workflow.NodeTypeTask, Status: workflow.NodeStatusRunning, CreatedAt: now.Add(time.Second), UpdatedAt: now.Add(time.Second)},
			"node-a": {
				ID: "node-a", Type: workflow.NodeTypeSplitTask, Status: workflow.NodeStatusCompleted, CreatedAt: now, UpdatedAt: now,
				ChildWorkflowIDs: []string{consignmentID + "--node-a--branch-1", consignmentID + "--node-a--branch-2"},
			},
			"node-c": {ID: "node-c", Type: workflow.NodeTypeTask, Status: workflow.NodeStatusNotStarted, CreatedAt: now.Add(2 * time.Second), UpdatedAt: now.Add(2 * time.Second)},
		},
		AuditTrail: []string{"workflow started"},
		Edges: []workflow.Edge{
			{ID: "e1", SourceID: "node-a", TargetID: "node-b", Condition: "declared_value > 500"},
		},
	}
	mockWM.On("GetStatus", ctx, consignmentID).Return(instance, nil)

	result, err := svc.GetEngineStatus(ctx, consignmentID)
	require.NoError(t, err)
	assert.Equal(t, consignmentID, result.ConsignmentID)
	assert.Equal(t, "RUNNING", result.Status)
	assert.Equal(t, []string{"workflow started"}, result.AuditTrail)
	assert.Equal(t, map[string]any{"declared_value": float64(1000)}, result.GlobalVariables)
	// node-c (NOT_STARTED) is filtered out — only node-a and node-b remain.
	require.Len(t, result.Nodes, 2)
	// Sorted by CreatedAt: node-a (earlier) before node-b.
	assert.Equal(t, "node-a", result.Nodes[0].ID)
	assert.Equal(t, "COMPLETED", result.Nodes[0].Status)
	assert.Equal(t, []string{consignmentID + "--node-a--branch-1", consignmentID + "--node-a--branch-2"}, result.Nodes[0].ChildWorkflowIDs)
	assert.Equal(t, "node-b", result.Nodes[1].ID)
	assert.Equal(t, "RUNNING", result.Nodes[1].Status)
	assert.Empty(t, result.Nodes[1].ChildWorkflowIDs)
	// Edges pass straight through, condition included — an admin resolving a GATEWAY parked on
	// "no matching conditions" needs to see exactly what each outgoing edge actually checks.
	require.Len(t, result.Edges, 1)
	assert.Equal(t, workflow.Edge{ID: "e1", SourceID: "node-a", TargetID: "node-b", Condition: "declared_value > 500"}, result.Edges[0])
	mockWM.AssertExpectations(t)
}

// A parked node's category and mappings pass straight through — the resolve UI reads them to tell
// an admin why the node parked and which workflow variables a RETRY reads or a COMPLETE writes.
func TestConsignmentService_GetEngineStatus_ParkContext(t *testing.T) {
	db, _ := setupTestDB(t)
	mockWM := new(MockWM)
	svc := mustNewService(t, db, nil, nil, nil, nil, nil)
	require.NoError(t, svc.RegisterWorkflowManager(mockWM))

	ctx := context.Background()
	consignmentID := uuid.NewString()
	now := time.Now()

	instance := &workflow.WorkflowInstance{
		ID:     consignmentID,
		Status: workflow.StatusRunning,
		NodeInfo: map[string]*workflow.NodeInfo{
			"review": {
				ID: "review:uuid-1", Type: workflow.NodeTypeTask, Status: workflow.NodeStatusAwaitingAdmin, CreatedAt: now, UpdatedAt: now,
				LastError:     "output mapping error: required task variable 'status' not found in task result",
				ParkCategory:  workflow.ParkCategoryOutputMapping,
				InputMapping:  map[string]string{"applicant?": "applicant_ref"},
				OutputMapping: map[string]string{"status": "review.outcome"},
			},
			"intake": {ID: "intake:uuid-2", Type: workflow.NodeTypeTask, Status: workflow.NodeStatusCompleted, CreatedAt: now.Add(time.Second), UpdatedAt: now.Add(time.Second)},
		},
	}
	mockWM.On("GetStatus", ctx, consignmentID).Return(instance, nil)

	result, err := svc.GetEngineStatus(ctx, consignmentID)
	require.NoError(t, err)
	require.Len(t, result.Nodes, 2)

	parked := result.Nodes[0]
	assert.Equal(t, "review:uuid-1", parked.ID)
	assert.Equal(t, "OUTPUT_MAPPING", parked.ParkCategory)
	assert.Equal(t, map[string]string{"applicant?": "applicant_ref"}, parked.InputMapping)
	assert.Equal(t, map[string]string{"status": "review.outcome"}, parked.OutputMapping)

	// A node that isn't parked carries none of it.
	assert.Empty(t, result.Nodes[1].ParkCategory)
	assert.Nil(t, result.Nodes[1].InputMapping)
	assert.Nil(t, result.Nodes[1].OutputMapping)
	mockWM.AssertExpectations(t)
}

func TestConsignmentService_GetEngineStatus_NotFound(t *testing.T) {
	db, _ := setupTestDB(t)
	mockWM := new(MockWM)
	svc := mustNewService(t, db, nil, nil, nil, nil, nil)
	require.NoError(t, svc.RegisterWorkflowManager(mockWM))

	ctx := context.Background()
	consignmentID := uuid.NewString()
	mockWM.On("GetStatus", ctx, consignmentID).Return((*workflow.WorkflowInstance)(nil), fmt.Errorf("%w: workflow not found", workflow.ErrWorkflowNotFound))

	_, err := svc.GetEngineStatus(ctx, consignmentID)
	assert.ErrorIs(t, err, ErrEngineWorkflowNotFound)
	mockWM.AssertExpectations(t)
}

func TestConsignmentService_GetEngineStatus_NoWorkflowManager(t *testing.T) {
	db, _ := setupTestDB(t)
	svc := mustNewService(t, db, nil, nil, nil, nil, nil)

	_, err := svc.GetEngineStatus(context.Background(), uuid.NewString())
	assert.Error(t, err)
}

// A TASK node's TaskWorkflowID is filled in from the task store, keyed off
// workflow.VarRootWorkflowID rather than the queried workflow ID itself — this matters when
// GetEngineStatus is called on a nested child branch, not just the consignment root.
func TestConsignmentService_GetEngineStatus_AttachesTaskWorkflowID(t *testing.T) {
	db, _ := setupTestDB(t)
	mockWM := new(MockWM)
	mockTaskStore := new(MockTaskStore)
	svc := mustNewService(t, db, nil, nil, nil, nil, mockTaskStore)
	require.NoError(t, svc.RegisterWorkflowManager(mockWM))

	ctx := context.Background()
	rootWorkflowID := uuid.NewString()
	childWorkflowID := uuid.NewString()

	instance := &workflow.WorkflowInstance{
		ID:     childWorkflowID,
		Status: workflow.StatusRunning,
		WorkflowVariables: map[string]any{
			workflow.VarRootWorkflowID: rootWorkflowID,
		},
		NodeInfo: map[string]*workflow.NodeInfo{
			"officer_review": {ID: "officer_review", Type: workflow.NodeTypeTask, Status: workflow.NodeStatusAwaitingAdmin},
			"gateway_node":   {ID: "gateway_node", Type: workflow.NodeTypeGateway, Status: workflow.NodeStatusCompleted},
		},
	}
	mockWM.On("GetStatus", ctx, childWorkflowID).Return(instance, nil)
	mockTaskStore.On("GetAllTasks", ctx, rootWorkflowID).Return([]store.TaskRecord{
		{TaskID: "officer_review", TaskWorkflowID: "task-wf-officer_review"},
	})

	result, err := svc.GetEngineStatus(ctx, childWorkflowID)
	require.NoError(t, err)
	require.Len(t, result.Nodes, 2)
	byID := map[string]EngineNodeDTO{result.Nodes[0].ID: result.Nodes[0], result.Nodes[1].ID: result.Nodes[1]}
	assert.Equal(t, "task-wf-officer_review", byID["officer_review"].TaskWorkflowID)
	// Not a TASK node — must not get a TaskWorkflowID even though nothing prevents the map
	// lookup from matching by coincidence.
	assert.Empty(t, byID["gateway_node"].TaskWorkflowID)
	mockWM.AssertExpectations(t)
	mockTaskStore.AssertExpectations(t)
}

func TestConsignmentService_GetTaskWorkflowEngineStatus(t *testing.T) {
	db, _ := setupTestDB(t)
	mockTaskWM := new(MockWM)
	svc := mustNewService(t, db, nil, nil, nil, nil, nil)
	require.NoError(t, svc.RegisterTaskWorkflowManager(mockTaskWM))

	ctx := context.Background()
	taskWorkflowID := "task-wf-officer_review"
	instance := &workflow.WorkflowInstance{
		ID:     taskWorkflowID,
		Status: workflow.StatusRunning,
		NodeInfo: map[string]*workflow.NodeInfo{
			"map_output": {ID: "map_output", Type: workflow.NodeTypeTask, Status: workflow.NodeStatusAwaitingAdmin, LastError: "output mapping error"},
		},
	}
	mockTaskWM.On("GetStatus", ctx, taskWorkflowID).Return(instance, nil)

	result, err := svc.GetTaskWorkflowEngineStatus(ctx, taskWorkflowID)
	require.NoError(t, err)
	assert.Equal(t, taskWorkflowID, result.ConsignmentID)
	require.Len(t, result.Nodes, 1)
	assert.Equal(t, "AWAITING_ADMIN", result.Nodes[0].Status)
	assert.Equal(t, "output mapping error", result.Nodes[0].LastError)
	mockTaskWM.AssertExpectations(t)
}

func TestConsignmentService_GetTaskWorkflowEngineStatus_NotFound(t *testing.T) {
	db, _ := setupTestDB(t)
	mockTaskWM := new(MockWM)
	svc := mustNewService(t, db, nil, nil, nil, nil, nil)
	require.NoError(t, svc.RegisterTaskWorkflowManager(mockTaskWM))

	ctx := context.Background()
	taskWorkflowID := "task-wf-missing"
	mockTaskWM.On("GetStatus", ctx, taskWorkflowID).Return((*workflow.WorkflowInstance)(nil), fmt.Errorf("%w: workflow not found", workflow.ErrWorkflowNotFound))

	_, err := svc.GetTaskWorkflowEngineStatus(ctx, taskWorkflowID)
	assert.ErrorIs(t, err, ErrEngineWorkflowNotFound)
	mockTaskWM.AssertExpectations(t)
}

func TestConsignmentService_GetTaskWorkflowEngineStatus_NoManager(t *testing.T) {
	db, _ := setupTestDB(t)
	svc := mustNewService(t, db, nil, nil, nil, nil, nil)

	_, err := svc.GetTaskWorkflowEngineStatus(context.Background(), "task-wf-officer_review")
	assert.Error(t, err)
}

func TestConsignmentService_RegisterTaskWorkflowManager_AlreadyRegistered(t *testing.T) {
	db, _ := setupTestDB(t)
	svc := mustNewService(t, db, nil, nil, nil, nil, nil)
	require.NoError(t, svc.RegisterTaskWorkflowManager(new(MockWM)))

	err := svc.RegisterTaskWorkflowManager(new(MockWM))
	assert.Error(t, err)
}

// TestConsignmentService_ResolveAdminIntervention_TranslatesCompositeNodeID guards the actual
// bug: an admin submits the composite "<template ID>:<uuid>" ID they were shown (EngineNodeDTO.ID
// — see NodeInfo.ID in core's GraphInterpreterWorkflow), but core's own signal routing
// (pendingAdminResolutions) keys on the plain template ID — nodeInfo's map key, not its .ID
// field. Without translating back, core silently drops the signal as routed to an unknown node
// and the admin's action never applies.
func TestConsignmentService_ResolveAdminIntervention_TranslatesCompositeNodeID(t *testing.T) {
	db, _ := setupTestDB(t)
	mockWM := new(MockWM)
	svc := mustNewService(t, db, nil, nil, nil, nil, nil)
	require.NoError(t, svc.RegisterWorkflowManager(mockWM))

	ctx := context.Background()
	workflowID := "task-wf-n1_apply:ca7ed707-1dba-43ca-94bf-10eddf00df3c"
	compositeNodeID := "officer_review:6aad0417-9a6d-4407-9509-2e51d8fcae99"

	instance := &workflow.WorkflowInstance{
		ID:     workflowID,
		Status: workflow.StatusRunning,
		NodeInfo: map[string]*workflow.NodeInfo{
			// Keyed by the plain template ID — NodeInfo.ID itself is the composite value an
			// admin actually sees and submits back (see workflow.go's GraphInterpreterWorkflow).
			"officer_review": {ID: compositeNodeID, Type: workflow.NodeTypeTask, Status: workflow.NodeStatusAwaitingAdmin},
		},
	}
	mockWM.On("GetStatus", ctx, workflowID).Return(instance, nil)
	mockWM.On("ResolveAdminIntervention", ctx, workflowID, "", mock.MatchedBy(func(sig workflow.AdminResolutionSignal) bool {
		return sig.NodeID == "officer_review"
	})).Return(nil)

	err := svc.ResolveAdminIntervention(ctx, workflowID, workflow.AdminResolutionSignal{
		NodeID: compositeNodeID,
		Action: workflow.AdminActionComplete,
		Reason: "activity already ran, supplying result manually",
	})
	require.NoError(t, err)
	mockWM.AssertExpectations(t)
}

func TestConsignmentService_ResolveAdminIntervention_UnknownNodeID(t *testing.T) {
	db, _ := setupTestDB(t)
	mockWM := new(MockWM)
	svc := mustNewService(t, db, nil, nil, nil, nil, nil)
	require.NoError(t, svc.RegisterWorkflowManager(mockWM))

	ctx := context.Background()
	workflowID := "consignment-1"
	instance := &workflow.WorkflowInstance{
		ID:     workflowID,
		Status: workflow.StatusRunning,
		NodeInfo: map[string]*workflow.NodeInfo{
			"officer_review": {ID: "officer_review:known-uuid", Type: workflow.NodeTypeTask, Status: workflow.NodeStatusAwaitingAdmin},
		},
	}
	mockWM.On("GetStatus", ctx, workflowID).Return(instance, nil)

	err := svc.ResolveAdminIntervention(ctx, workflowID, workflow.AdminResolutionSignal{
		NodeID: "officer_review:stale-uuid-from-a-prior-run",
		Action: workflow.AdminActionRetry,
		Reason: "retry",
	})
	assert.ErrorIs(t, err, ErrNodeNotParked)
	mockWM.AssertExpectations(t)
}

func TestConsignmentService_ResolveAdminIntervention_NodeNotAwaitingAdmin(t *testing.T) {
	db, _ := setupTestDB(t)
	mockWM := new(MockWM)
	svc := mustNewService(t, db, nil, nil, nil, nil, nil)
	require.NoError(t, svc.RegisterWorkflowManager(mockWM))

	ctx := context.Background()
	workflowID := "consignment-1"
	instance := &workflow.WorkflowInstance{
		ID:     workflowID,
		Status: workflow.StatusRunning,
		NodeInfo: map[string]*workflow.NodeInfo{
			"officer_review": {ID: "officer_review:some-uuid", Type: workflow.NodeTypeTask, Status: workflow.NodeStatusRunning},
		},
	}
	mockWM.On("GetStatus", ctx, workflowID).Return(instance, nil)

	err := svc.ResolveAdminIntervention(ctx, workflowID, workflow.AdminResolutionSignal{
		NodeID: "officer_review:some-uuid",
		Action: workflow.AdminActionRetry,
		Reason: "retry",
	})
	assert.ErrorIs(t, err, ErrNodeNotParked)
	mockWM.AssertExpectations(t)
}

func TestConsignmentService_ResolveAdminIntervention_GatewayCompleteUnsupported(t *testing.T) {
	db, _ := setupTestDB(t)
	mockWM := new(MockWM)
	svc := mustNewService(t, db, nil, nil, nil, nil, nil)
	require.NoError(t, svc.RegisterWorkflowManager(mockWM))

	ctx := context.Background()
	workflowID := "consignment-1"
	instance := &workflow.WorkflowInstance{
		ID:     workflowID,
		Status: workflow.StatusRunning,
		NodeInfo: map[string]*workflow.NodeInfo{
			"gw1": {ID: "gw1:some-uuid", Type: workflow.NodeTypeGateway, Status: workflow.NodeStatusAwaitingAdmin},
		},
	}
	mockWM.On("GetStatus", ctx, workflowID).Return(instance, nil)

	err := svc.ResolveAdminIntervention(ctx, workflowID, workflow.AdminResolutionSignal{
		NodeID: "gw1:some-uuid",
		Action: workflow.AdminActionComplete,
		Reason: "complete",
	})
	assert.ErrorIs(t, err, ErrAdminActionUnsupportedForGateway)
	// ResolveAdminIntervention on the manager must never be called for a rejected action — no
	// expectation was set for it above, so testify would panic on an unexpected call.
	mockWM.AssertExpectations(t)
}
