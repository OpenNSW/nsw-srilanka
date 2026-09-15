package consignment

import (
	"context"
	"testing"
	"time"

	"fmt"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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
