export type WorkflowStepType = 'TRADER_FORM' | 'OGA_APPLICATION' | 'SYSTEM_WAIT'

export type WorkflowStepStatus = 'COMPLETED' | 'ACTIVE' | 'LOCKED' | 'PENDING'

export interface WorkflowStepConfig {
  formId?: string
  agency?: string
  service?: string
  requestURL?: string
  event?: string
}

export interface WorkflowStep {
  id: string
  type: WorkflowStepType
  config: WorkflowStepConfig
  dependsOn: string[]
}

export interface WorkflowDefinition {
  workflowId: string
  version: string
  steps: WorkflowStep[]
}

export interface Workflow {
  id: string
  name: string
  description: string
  type: 'import' | 'export'
  hsCode: string
  definition?: WorkflowDefinition
}

export interface WorkflowQueryParams {
  hs_code: string
}