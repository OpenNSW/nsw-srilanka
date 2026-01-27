export type TradeFlow = 'IMPORT' | 'EXPORT'

export type ConsignmentState = 'IN_PROGRESS' | 'REQUIRES_REWORK' | 'FINISHED'

export type StepStatus = 'READY' | 'LOCKED' | 'IN_PROGRESS' | 'COMPLETED' | 'REJECTED'

export type StepType = 'SIMPLE_FORM' | 'WAIT_FOR_EVENT'

export interface ConsignmentStep {
  stepId: string
  type: StepType
  taskId: string
  status: StepStatus
  dependsOn: string[]
}

export interface ConsignmentItem {
  hsCodeID: string
  steps: ConsignmentStep[]
}

export interface Consignment {
  id: string
  tradeFlow: TradeFlow
  traderId: string
  state: ConsignmentState
  items: ConsignmentItem[]
  createdAt: string
  updatedAt: string
}

export interface CreateConsignmentItemRequest {
  hsCodeId: string
  metadata: Record<string, unknown>
  workflowTemplateId: string
}

export interface CreateConsignmentRequest {
  tradeFlow: TradeFlow
  traderId: string
  items: CreateConsignmentItemRequest[]
}

export interface CreateConsignmentResponse extends Consignment {}