package consignment

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"

	workflow "github.com/OpenNSW/core/workflow"
)

// EngineNodeDTO is a presentational view of one DAG node's raw engine state, as
// tracked by the workflow engine itself (workflow.NodeInfo) — distinct from
// WorkflowNodeResponseDTO, which is derived from the task store and reflects
// business/task semantics instead.
type EngineNodeDTO struct {
	ID             string    `json:"id"`
	Type           string    `json:"type"`
	GatewayType    string    `json:"gateway_type,omitempty"`
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
	// TaskWorkflowID is set only for a TASK node whose task has actually started (there's a
	// matching store.TaskRecord) — the Temporal workflow ID of the independent per-task ("micro")
	// workflow spawned to fulfill it, on a separate manager/task queue from this node's own
	// workflow (see Service.taskWm). Fetch it via the task-workflow engine-status endpoint to
	// drill down — a node parked inside it is otherwise invisible here, since from this node's
	// own workflow's point of view the TASK node is just an Activity pending completion.
	TaskWorkflowID string `json:"task_workflow_id,omitempty"`
}

// EngineStatusDTO is the root workflow's raw engine state for a consignment.
type EngineStatusDTO struct {
	ConsignmentID string          `json:"consignment_id"`
	Status        string          `json:"status"`
	Nodes         []EngineNodeDTO `json:"nodes"`
	AuditTrail    []string        `json:"audit_trail"`
	// GlobalVariables is the workflow instance's shared, dynamic business data
	// (workflow.WorkflowInstance.WorkflowVariables) — workflow-wide, not per node; every node in
	// this workflow sees the same snapshot. May hold business/PII data, hence ConsignmentAdminRead
	// rather than the trader/CHA-facing ConsignmentRead scope.
	GlobalVariables map[string]any `json:"global_variables,omitempty"`
}

// ErrEngineWorkflowNotFound is returned by GetEngineStatus when no workflow
// execution exists for the given ID on the registered workflow manager.
var ErrEngineWorkflowNotFound = errors.New("workflow execution not found")

// GetEngineStatus returns the raw engine state for workflowID — the consignment's root workflow,
// or (queried the same way, via this same endpoint) a nested child spawned by a SPLIT_TASK/
// BATCH_SPLIT/PARALLEL_SPLIT node — straight from the registered workflow.Manager (Temporal
// today, but Manager is an interface so this stays agnostic to whatever runtime backs it).
// Unlike GetConsignmentByID, it performs no trader/CHA ownership check — see the TODO on the
// route wiring in router.go.
//
// Each TASK node's DTO is additionally enriched with TaskWorkflowID, when its task has actually
// started — resolved via the task store rather than reconstructed from core/taskflow's
// "task-wf-"+nodeID ID scheme, so this doesn't hardcode a convention private to that package.
func (s *Service) GetEngineStatus(ctx context.Context, workflowID string) (*EngineStatusDTO, error) {
	if s.wm == nil {
		return nil, fmt.Errorf("no workflow manager registered for ConsignmentService")
	}

	instance, err := s.wm.GetStatus(ctx, workflowID)
	if err != nil {
		if errors.Is(err, workflow.ErrWorkflowNotFound) {
			return nil, ErrEngineWorkflowNotFound
		}
		return nil, fmt.Errorf("failed to get engine status for workflow %s: %w", workflowID, err)
	}

	dto := buildEngineStatusDTO(workflowID, instance)
	s.attachTaskWorkflowIDs(ctx, instance, dto.Nodes)
	return dto, nil
}

// GetTaskWorkflowEngineStatus returns the raw engine state for taskWorkflowID — the independent
// per-task ("micro") workflow a TASK node spawned to fulfill it (see EngineNodeDTO.TaskWorkflowID)
// — from the registered task-workflow manager, a separate Temporal task queue from the one
// GetEngineStatus queries. A node parked for admin intervention inside a task workflow is
// otherwise invisible: from its owning TASK node's own workflow, the node is just an Activity
// pending completion.
func (s *Service) GetTaskWorkflowEngineStatus(ctx context.Context, taskWorkflowID string) (*EngineStatusDTO, error) {
	if s.taskWm == nil {
		return nil, fmt.Errorf("no task workflow manager registered for ConsignmentService")
	}

	instance, err := s.taskWm.GetStatus(ctx, taskWorkflowID)
	if err != nil {
		if errors.Is(err, workflow.ErrWorkflowNotFound) {
			return nil, ErrEngineWorkflowNotFound
		}
		return nil, fmt.Errorf("failed to get engine status for task workflow %s: %w", taskWorkflowID, err)
	}

	return buildEngineStatusDTO(taskWorkflowID, instance), nil
}

// buildEngineStatusDTO converts a *workflow.WorkflowInstance into the presentational DTO shared
// by GetEngineStatus and GetTaskWorkflowEngineStatus — the two differ only in which
// workflow.Manager they queried and whether TaskWorkflowID enrichment applies.
func buildEngineStatusDTO(workflowID string, instance *workflow.WorkflowInstance) *EngineStatusDTO {
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
			GatewayType:      string(n.GatewayType),
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
		ConsignmentID:   workflowID,
		Status:          string(instance.Status),
		Nodes:           nodes,
		AuditTrail:      instance.AuditTrail,
		GlobalVariables: instance.WorkflowVariables,
	}
}

// attachTaskWorkflowIDs fills in TaskWorkflowID on each TASK-type node in nodes, when its task
// has started. Task records are stored keyed by the true consignment root workflow ID
// (store.TaskRecord.RootWorkflowID) even for a node nested several levels deep in a
// BATCH_SPLIT/PARALLEL_SPLIT branch — so the root ID is read off instance.WorkflowVariables
// (workflow.VarRootWorkflowID, propagated by the engine to every level of nesting) rather than
// assumed to be the workflowID GetEngineStatus was actually called with. Best-effort: if
// taskStore is nil or the root ID can't be determined, nodes are left without TaskWorkflowID
// rather than failing the whole engine-status read.
func (s *Service) attachTaskWorkflowIDs(ctx context.Context, instance *workflow.WorkflowInstance, nodes []EngineNodeDTO) {
	if s.taskStore == nil {
		return
	}
	rootWorkflowID, _ := instance.WorkflowVariables[workflow.VarRootWorkflowID].(string)
	if rootWorkflowID == "" {
		return
	}

	byNodeID := make(map[string]string, len(nodes))
	for _, rec := range s.taskStore.GetAllTasks(ctx, rootWorkflowID) {
		if rec.TaskWorkflowID != "" {
			byNodeID[rec.TaskID] = rec.TaskWorkflowID
		}
	}
	for i := range nodes {
		if nodes[i].Type != string(workflow.NodeTypeTask) {
			continue
		}
		if taskWorkflowID, ok := byNodeID[nodes[i].ID]; ok {
			nodes[i].TaskWorkflowID = taskWorkflowID
		}
	}
}
