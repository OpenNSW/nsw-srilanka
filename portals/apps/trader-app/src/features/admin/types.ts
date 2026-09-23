// Raw engine state as reported by the workflow manager (see
// OpenNSW/core workflow.NodeInfo / workflow.WorkflowInstance) — distinct from
// the trader-facing WorkflowNodeState in features/consignment/types.ts, which
// reflects task-store/business state instead.
export type EngineNodeStatus = 'NOT_STARTED' | 'RUNNING' | 'COMPLETED' | 'FAILED' | 'AWAITING_ADMIN'

// Why a node parked in AWAITING_ADMIN (workflow.ParkCategory on the backend) — lets the resolve
// screen say what went wrong without parsing last_error.
export type ParkCategory =
  | 'INPUT_MAPPING'
  | 'OUTPUT_MAPPING'
  | 'TASK_FAILURE'
  | 'GATEWAY_CONDITION'
  | 'SPLIT_DATA'
  | 'CHILD_FAILURE'
  | 'DEFINITION_ERROR'
  | 'UNKNOWN'

export interface EngineNode {
  id: string
  type: string
  // Only present when type is 'GATEWAY' — which kind (EXCLUSIVE_SPLIT, PARALLEL_SPLIT,
  // EXCLUSIVE_JOIN, PARALLEL_JOIN, BATCH_SPLIT, BATCH_JOIN).
  gateway_type?: string
  task_template_id?: string
  status: EngineNodeStatus
  last_error?: string
  // Set together with last_error, only while the node is AWAITING_ADMIN.
  park_category?: ParkCategory
  // The node's mappings, present only while it is parked. input_mapping is keyed by the workflow
  // variable it reads (a trailing "?" marks it optional) with the task input it fills as the
  // value; output_mapping is keyed by the task result field (same "?" rule) with the workflow
  // variable it writes as the value. So a RETRY fixes input_mapping keys, and a COMPLETE patch
  // supplies output_mapping values.
  input_mapping?: Record<string, string>
  output_mapping?: Record<string, string>
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
  // already happened, so COMPLETE (supply its output in the patch) is usually preferable to RETRY
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

// Which family of admin routes addresses a workflow instance. 'consignment' covers a consignment's
// root workflow and its child-branch workflows; 'task' is a TASK node's own task workflow
// (EngineNode.task_workflow_id), a separate ID space on a separate workflow manager. The
// engine-status and resolve endpoints are split the same way.
export type AdminWorkflowKind = 'consignment' | 'task'

// How an admin resolves a node parked in AWAITING_ADMIN (see core/workflow.AdminResolutionAction
// on the backend). COMPLETE is rejected by the engine for GATEWAY nodes — a gateway's routing
// can't be completed without bypassing its own condition logic.
export type AdminResolutionAction = 'RETRY' | 'COMPLETE' | 'ABORT'

export interface AdminResolutionRequest {
  action: AdminResolutionAction
  // A patch, not the full set of global variables: dotted paths (e.g. "review.outcome") to the
  // values to write, applied before RETRY re-runs the node or COMPLETE marks it done. A map value
  // is merged into an existing map at that path; any other value replaces it. What it writes is
  // workflow-wide and persists, so it affects later nodes too. Only RETRY and COMPLETE use it.
  global_variables_patch?: Record<string, unknown>
  reason: string
}
