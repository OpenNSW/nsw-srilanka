export type WorkflowStepType = 'SIMPLE_FORM' | 'WAIT_FOR_EVENT'

export interface WorkflowStepConfig {
  formId?: string
  agency?: string
  service?: string
  event?: string
}

export interface WorkflowStep {
  stepId: string
  type: WorkflowStepType
  config: WorkflowStepConfig
  dependsOn: string[]
}

export interface WorkflowTemplate {
  id: string
  createdAt: string
  updatedAt: string
  version: string
  steps: WorkflowStep[]
}

export interface Workflow {
  id: string
  name: string
  type: 'import' | 'export'
  steps: WorkflowStep[]
}

export interface WorkflowQueryParams {
  hs_code: string
}