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
}

export type EngineWorkflowStatus = 'RUNNING' | 'COMPLETED' | 'FAILED'

export interface EngineStatus {
  consignment_id: string
  status: EngineWorkflowStatus
  nodes: EngineNode[]
  audit_trail: string[]
}
