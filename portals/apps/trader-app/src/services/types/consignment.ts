export type TradeFlow = 'IMPORT' | 'EXPORT'

export type ConsignmentState = 'IN_PROGRESS' | 'REQUIRES_REWORK' | 'FINISHED' | 'COMPLETED'

export type WorkflowNodeState = 'READY' | 'LOCKED' | 'IN_PROGRESS' | 'COMPLETED' | 'REJECTED'

export type StepType = 'SIMPLE_FORM' | 'WAIT_FOR_EVENT'

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
  flow: TradeFlow
  traderId: string
  state: ConsignmentState
  items: ConsignmentItem[]
  createdAt: string
  updatedAt: string
  workflowNodeCount: number
  completedWorkflowNodeCount: number
}

export interface ConsignmentDetail {
  id: string
  flow: TradeFlow
  traderId: string
  state: ConsignmentState
  items: ConsignmentItem[]
  globalContext: GlobalContext
  createdAt: string
  updatedAt: string
  workflowNodes: WorkflowNode[]
}

// Deprecated: Use ConsignmentDetail or ConsignmentSummary
export type Consignment = ConsignmentDetail

export interface CreateConsignmentItemRequest {
  hsCodeId: string
}

export interface CreateConsignmentRequest {
  flow: TradeFlow
  items: CreateConsignmentItemRequest[]
}

export type CreateConsignmentResponse = ConsignmentDetail

import type { PaginatedResponse } from './common'

export type ConsignmentListResult = PaginatedResponse<ConsignmentSummary>