// Raw engine state as reported by the workflow manager (see
// OpenNSW/core workflow.NodeInfo / workflow.WorkflowInstance) — distinct from
// the trader-facing WorkflowNodeState in features/consignment/types.ts, which
// reflects task-store/business state instead.
export type EngineNodeStatus = 'NOT_STARTED' | 'RUNNING' | 'COMPLETED' | 'FAILED' | 'AWAITING_ADMIN'

export interface EngineNode {
  id: string
  type: string
  // Only present when type is 'GATEWAY' — which kind (EXCLUSIVE_SPLIT, PARALLEL_SPLIT,
  // EXCLUSIVE_JOIN, PARALLEL_JOIN, BATCH_SPLIT, BATCH_JOIN).
  gateway_type?: string
  task_template_id?: string
  status: EngineNodeStatus
  last_error?: string
  created_at: string
  updated_at: string
  // IDs of any child workflow executions this node spawned (SPLIT_TASK / BATCH_SPLIT). Each can
  // be looked up via the same engine-status endpoint to drill down, whether or not it has since
  // completed.
  child_workflow_ids?: string[]
  // Set only for a TASK node whose task has actually started — the workflow ID of the
  // independent per-task ("micro") workflow spawned to fulfill it. A separate ID space/manager
  // from child_workflow_ids above: fetch it via getTaskWorkflowEngineStatus, not
  // getConsignmentEngineStatus, to drill down (see EngineNodeDTO.TaskWorkflowID on the backend).
  task_workflow_id?: string
  // The most recent raw Activity result for a TASK node, if its Activity already ran — cleared
  // once the node fully completes. When present on a node AWAITING_ADMIN, the Activity has
  // already happened, so OVERRIDE (supply data in its place) is usually preferable to RETRY
  // (re-runs it) — see ResolveAdminInterventionForm.
  cached_task_result?: Record<string, unknown>
}

export type EngineWorkflowStatus = 'RUNNING' | 'COMPLETED' | 'FAILED'

// This workflow instance's own graph connections (workflow.Edge on the backend) — source_id/
// target_id already resolved to the composite node IDs in EngineStatus.nodes[i].id. condition is
// the raw expr-lang expression evaluated against global_variables, verbatim.
export interface EngineEdge {
  id: string
  source_id: string
  target_id: string
  condition?: string
}

export interface EngineStatus {
  consignment_id: string
  status: EngineWorkflowStatus
  nodes: EngineNode[]
  audit_trail: string[]
  // Workflow-wide shared/dynamic business data (workflow.WorkflowInstance.WorkflowVariables on
  // the backend) — the same snapshot regardless of which node you're looking at.
  global_variables?: Record<string, unknown>
  edges?: EngineEdge[]
}

// How an admin resolves a node parked in AWAITING_ADMIN (see core/workflow.AdminResolutionAction
// on the backend). SKIP and OVERRIDE are rejected by the engine for GATEWAY nodes — a gateway's
// routing can't be skipped/overridden without bypassing its own condition logic.
export type AdminResolutionAction = 'RETRY' | 'OVERRIDE' | 'SKIP' | 'ABORT'

export interface AdminResolutionRequest {
  action: AdminResolutionAction
  overrides?: Record<string, unknown>
  reason: string
}
