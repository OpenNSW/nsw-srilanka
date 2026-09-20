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
//
// TODO: ID, LastError, CreatedAt, UpdatedAt, ChildWorkflowIDs and CachedTaskResult are plain
// copies of workflow.NodeInfo fields; revisit whether they need their own struct.
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
	// matching store.TaskRecord) — the workflow ID of the independent per-task ("micro") workflow
	// spawned to fulfill it, on a separate manager/task queue from this node's own workflow (see
	// Service.taskWm). Fetch it via the task-workflow engine-status endpoint to drill down — a
	// node parked inside it is otherwise invisible here, since from this node's own workflow's
	// point of view the TASK node is just pending completion.
	TaskWorkflowID string `json:"task_workflow_id,omitempty"`
	// CachedTaskResult is the raw Activity result of a TASK node whose Activity already ran, until
	// the node completes. It shows an admin what came back before choosing RETRY or COMPLETE.
	CachedTaskResult map[string]any `json:"cached_task_result,omitempty"`
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
	// Edges are the workflow's graph connections (workflow.WorkflowInstance.Edges). Source and
	// target are the composite IDs in Nodes[i].ID; conditions are verbatim. Lets an admin see what
	// a parked GATEWAY's outgoing edges check.
	Edges []workflow.Edge `json:"edges,omitempty"`
}

// ErrEngineWorkflowNotFound is returned by GetEngineStatus when no workflow
// execution exists for the given ID on the registered workflow manager.
var ErrEngineWorkflowNotFound = errors.New("workflow execution not found")

// ErrNodeNotParked is returned by ResolveAdminIntervention when the node isn't AWAITING_ADMIN
// (wrong ID, or already resolved). Core silently drops a signal for such a node, so we check first.
var ErrNodeNotParked = errors.New("node is not currently awaiting admin intervention")

// ErrAdminActionUnsupportedForGateway is returned for COMPLETE on a GATEWAY, which core would only
// log and re-park: a gateway's routing can't be bypassed.
var ErrAdminActionUnsupportedForGateway = errors.New("complete is not supported for GATEWAY nodes; use retry or abort")

// ErrAdminInterventionUnsupported is returned when the workflow manager doesn't implement
// workflow.AdminInterventionResolver.
var ErrAdminInterventionUnsupported = errors.New("workflow manager does not support resolving admin interventions")

// GetEngineStatus returns the raw engine state for workflowID — the consignment's root workflow,
// or (queried the same way, via this same endpoint) a nested child spawned by a SPLIT_TASK/
// BATCH_SPLIT/PARALLEL_SPLIT node — straight from the registered workflow.Manager, an interface
// so this stays agnostic to whatever engine actually backs it. Unlike GetConsignmentByID, it
// performs no trader/CHA ownership check — see the TODO on the route wiring in router.go.
//
// Each TASK node's DTO is additionally enriched with TaskWorkflowID, when its task has actually
// started.
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
	// Fetches every task record for the whole consignment, not just the ones under workflowID's
	// own branch — the task store only supports filtering by root workflow ID (see
	// attachTaskWorkflowIDs), so a nested child-branch call re-fetches the same consignment-wide
	// list. Acceptable for an admin screen opened manually and rarely; would need a narrower
	// query on the task store itself (TaskRecord.ParentWorkflowID already holds the more specific
	// ID, but nothing queries by it) if this ever shows up as an actual cost.
	s.attachTaskWorkflowIDs(ctx, instance, dto.Nodes)
	return dto, nil
}

// GetTaskWorkflowEngineStatus returns the raw engine state for taskWorkflowID — the independent
// per-task ("micro") workflow a TASK node spawned to fulfill it (see EngineNodeDTO.TaskWorkflowID)
// — from the registered task-workflow manager, a separate manager/task queue from the one
// GetEngineStatus queries.
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
			CachedTaskResult: n.CachedTaskResult,
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
		Edges:           instance.Edges,
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

// ResolveAdminIntervention sends an admin's decision (RETRY/COMPLETE/ABORT) to a node parked
// in AWAITING_ADMIN on workflowID, which can be a root, child-branch or task workflow. It first
// confirms the node is parked and that a GATEWAY isn't asked to COMPLETE.
//
// sig.NodeID arrives as the composite "<template ID>:<uuid>" (EngineNodeDTO.ID), but core routes
// signals by the plain template ID, so it is translated back before the check and the signal.
//
// The parked check and the signal are not atomic: the manager's ResolveAdminIntervention is a
// fire-and-forget Temporal signal, and core drops a signal for a node that is no longer parked
// without reporting it. If two admins resolve the same node at once, both can pass the check and
// both get success, though only the first signal takes effect. We accept this because admin
// resolution is expected to be a single admin acting at a time, not concurrent. TODO: fix this
// properly in core with an acknowledged Temporal Update that rejects a node that is no longer
// parked, then map that rejection to ErrNodeNotParked so the stale request gets a 409.
func (s *Service) ResolveAdminIntervention(ctx context.Context, workflowID string, sig workflow.AdminResolutionSignal) error {
	if s.wm == nil {
		return fmt.Errorf("no workflow manager registered for ConsignmentService")
	}
	resolver, ok := s.wm.(workflow.AdminInterventionResolver)
	if !ok {
		return ErrAdminInterventionUnsupported
	}

	instance, err := s.wm.GetStatus(ctx, workflowID)
	if err != nil {
		if errors.Is(err, workflow.ErrWorkflowNotFound) {
			return ErrEngineWorkflowNotFound
		}
		return fmt.Errorf("failed to verify node %s is parked on workflow %s: %w", sig.NodeID, workflowID, err)
	}
	templateID, node, ok := findNodeByCompositeID(instance.NodeInfo, sig.NodeID)
	if !ok || node.Status != workflow.NodeStatusAwaitingAdmin {
		return ErrNodeNotParked
	}
	if node.Type == workflow.NodeTypeGateway && sig.Action == workflow.AdminActionComplete {
		return ErrAdminActionUnsupportedForGateway
	}
	sig.NodeID = templateID

	if err := resolver.ResolveAdminIntervention(ctx, workflowID, "", sig); err != nil {
		return fmt.Errorf("failed to resolve admin intervention for workflow %s node %s: %w", workflowID, sig.NodeID, err)
	}
	return nil
}

// findNodeByCompositeID finds a node by its composite NodeInfo.ID and returns its template ID (the
// nodeInfo map key). A scan is fine: nodeInfo is small.
func findNodeByCompositeID(nodeInfo map[string]*workflow.NodeInfo, compositeID string) (templateID string, node *workflow.NodeInfo, ok bool) {
	for id, n := range nodeInfo {
		if n.ID == compositeID {
			return id, n, true
		}
	}
	return "", nil, false
}
