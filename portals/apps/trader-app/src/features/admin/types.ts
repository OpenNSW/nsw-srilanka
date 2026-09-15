// Raw engine state as reported by the workflow manager (see
// OpenNSW/core workflow.NodeInfo / workflow.WorkflowInstance) — distinct from
// the trader-facing WorkflowNodeState in features/consignment/types.ts, which
// reflects task-store/business state instead.
export type EngineNodeStatus = 'NOT_STARTED' | 'RUNNING' | 'COMPLETED' | 'FAILED' | 'AWAITING_ADMIN'

export interface EngineNode {
  id: string
  type: string
  task_template_id?: string
  status: EngineNodeStatus
  last_error?: string
  created_at: string
  updated_at: string
  // IDs of any child workflow executions this node spawned (SPLIT_TASK / BATCH_SPLIT). Each can
  // be looked up via the same engine-status endpoint to drill down, whether or not it has since
  // completed.
  child_workflow_ids?: string[]
}

export type EngineWorkflowStatus = 'RUNNING' | 'COMPLETED' | 'FAILED'

export interface EngineStatus {
  consignment_id: string
  status: EngineWorkflowStatus
  nodes: EngineNode[]
  audit_trail: string[]
  // Workflow-wide shared/dynamic business data (workflow.WorkflowInstance.WorkflowVariables on
  // the backend) — the same snapshot regardless of which node you're looking at.
  global_variables?: Record<string, unknown>
}
