import type { PaginatedResponse } from '@/services/types/common'

export type WorkflowStepType = 'SIMPLE_FORM' | 'WAIT_FOR_EVENT' | 'PAYMENT'

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

export interface WorkflowResponse {
  import: Workflow[]
  export: Workflow[]
}

export interface CHA {
  id: string
  name: string
  description: string
  email?: string
}

export type TradeFlow = 'IMPORT' | 'EXPORT'

export type ConsignmentState = 'INITIALIZED' | 'IN_PROGRESS' | 'FAILED' | 'FINISHED'

export type WorkflowNodeState = 'READY' | 'LOCKED' | 'IN_PROGRESS' | 'COMPLETED' | 'FAILED'

export type StepType = 'SIMPLE_FORM' | 'WAIT_FOR_EVENT' | 'PAYMENT' | 'START' | 'END' | 'GATEWAY' | 'END_NODE'

export interface GlobalContext {
  consigneeAddress: string
  consigneeName: string
  countryOfDestination: string
  countryOfOrigin: string
  invoiceDate: string
  invoiceNumber: string
}

export interface HSCodeDetails {
  hsCodeId: string
  hsCode: string
  description: string
  category: string
}

export interface WorkflowNodeTemplate {
  name: string
  description: string
  type: StepType
}

export interface WorkflowNode {
  id: string
  createdAt: string
  updatedAt: string
  workflowNodeTemplate: WorkflowNodeTemplate
  state: WorkflowNodeState
  extendedState?: string
  depends_on: string[]
}

export interface ConsignmentItem {
  hsCode: HSCodeDetails
}

export interface ConsignmentSummary {
  id: string
  name?: string
  flow: TradeFlow
  traderId: string
  chaId?: string
  state: ConsignmentState
  items: ConsignmentItem[]
  createdAt: string
  updatedAt: string
  workflowNodeCount: number
  completedWorkflowNodeCount: number
}

export interface ConsignmentDetail {
  id: string
  name?: string
  flow: TradeFlow
  traderId: string
  chaId?: string
  state: ConsignmentState
  items: ConsignmentItem[]
  globalContext: GlobalContext
  createdAt: string
  updatedAt: string
  workflowNodes: WorkflowNode[]
}

// Deprecated: Use ConsignmentDetail or ConsignmentSummary
export type Consignment = ConsignmentDetail

export type CreateConsignmentResponse = ConsignmentDetail

export type ConsignmentListResult = PaginatedResponse<ConsignmentSummary>

// PreConsignment types
export type PreConsignmentState = 'LOCKED' | 'READY' | 'IN_PROGRESS' | 'COMPLETED'

export interface PreConsignmentTemplate {
  id: string
  name: string
  description: string
  dependsOn: string[]
}

export interface PreConsignmentInstance {
  id: string
  traderId: string
  state: PreConsignmentState
  traderContext: Record<string, unknown>
  createdAt: string
  updatedAt: string
  preConsignmentTemplate: PreConsignmentTemplate
  workflowNodes: WorkflowNode[]
}

export interface TraderPreConsignmentItem {
  id: string
  name: string
  description: string
  state: PreConsignmentState
  dependsOn: string[]
  preConsignment?: PreConsignmentInstance
  preConsignmentTemplate?: PreConsignmentTemplate
}

export type TraderPreConsignmentsResponse = PaginatedResponse<TraderPreConsignmentItem>

export type PreConsignmentListApiResponse = PreConsignmentInstance[] | TraderPreConsignmentsResponse

export interface CreatePreConsignmentRequest {
  preConsignmentTemplateId: string
}
