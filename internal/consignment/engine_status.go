package consignment

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"

	"go.temporal.io/api/serviceerror"

	workflow "github.com/OpenNSW/core/workflow"
)

// EngineNodeDTO is a presentational view of one DAG node's raw engine state, as
// tracked by the workflow engine itself (workflow.NodeInfo) — distinct from
// WorkflowNodeResponseDTO, which is derived from the task store and reflects
// business/task semantics instead.
type EngineNodeDTO struct {
	ID             string    `json:"id"`
	Type           string    `json:"type"`
	TaskTemplateID string    `json:"task_template_id,omitempty"`
	Status         string    `json:"status"`
	LastError      string    `json:"last_error,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
	// ChildWorkflowIDs lists any child workflow executions spawned by this node (SPLIT_TASK /
	// BATCH_SPLIT). Each ID can be fetched via the same engine-status endpoint (it treats the
	// path parameter as a workflow ID, not a consignment record) to drill down, regardless of
	// whether that child has since completed.
	ChildWorkflowIDs []string `json:"child_workflow_ids,omitempty"`
}

// EngineStatusDTO is the root workflow's raw engine state for a consignment.
// It deliberately omits WorkflowVariables (may hold business/PII data) — this
// view exists for ops visibility into node progress, not for reading business data.
type EngineStatusDTO struct {
	ConsignmentID string          `json:"consignment_id"`
	Status        string          `json:"status"`
	Nodes         []EngineNodeDTO `json:"nodes"`
	AuditTrail    []string        `json:"audit_trail"`
}

// ErrEngineWorkflowNotFound is returned by GetEngineStatus when no workflow
// execution exists for the given ID on the registered workflow manager.
var ErrEngineWorkflowNotFound = errors.New("workflow execution not found")

// GetEngineStatus returns the root workflow's raw engine state for consignmentID,
// straight from the registered workflow.Manager (Temporal today, but Manager is
// an interface so this stays agnostic to whatever runtime backs it). Unlike
// GetConsignmentByID, it performs no trader/CHA ownership check — see the TODO
// on the route wiring in router.go.
func (s *Service) GetEngineStatus(ctx context.Context, consignmentID string) (*EngineStatusDTO, error) {
	if s.wm == nil {
		return nil, fmt.Errorf("no workflow manager registered for ConsignmentService")
	}

	instance, err := s.wm.GetStatus(ctx, consignmentID)
	if err != nil {
		var notFound *serviceerror.NotFound
		if errors.As(err, &notFound) {
			return nil, ErrEngineWorkflowNotFound
		}
		return nil, fmt.Errorf("failed to get engine status for consignment %s: %w", consignmentID, err)
	}

	nodes := make([]EngineNodeDTO, 0, len(instance.NodeInfo))
	for _, n := range instance.NodeInfo {
		// A DAG can define far more nodes than are ever relevant to look at — omit ones the
		// interpreter hasn't reached yet, since "not started" carries no ops-actionable signal.
		if n.Status == workflow.NodeStatusNotStarted {
			continue
		}
		nodes = append(nodes, EngineNodeDTO{
			ID:               n.ID,
			Type:             string(n.Type),
			TaskTemplateID:   n.TaskTemplateID,
			Status:           string(n.Status),
			LastError:        n.LastError,
			CreatedAt:        n.CreatedAt,
			UpdatedAt:        n.UpdatedAt,
			ChildWorkflowIDs: n.ChildWorkflowIDs,
		})
	}
	// NodeInfo is a map; sort for a stable, readable response instead of
	// random iteration order.
	sort.Slice(nodes, func(i, j int) bool {
		if !nodes[i].CreatedAt.Equal(nodes[j].CreatedAt) {
			return nodes[i].CreatedAt.Before(nodes[j].CreatedAt)
		}
		return nodes[i].ID < nodes[j].ID
	})

	return &EngineStatusDTO{
		ConsignmentID: consignmentID,
		Status:        string(instance.Status),
		Nodes:         nodes,
		AuditTrail:    instance.AuditTrail,
	}, nil
}
