export interface Consignment {
  id: string
  hsCode: string
  hsCodeDescription: string
  workflowId: string
  workflowName: string
  workflowType: 'import' | 'export'
  status: 'draft' | 'pending' | 'in_progress' | 'completed' | 'rejected'
  currentStepId?: string
  createdAt: string
  updatedAt: string
}

export interface CreateConsignmentRequest {
  hsCode: string
  hsCodeDescription: string
  workflowId: string
  workflowName: string
  workflowType: 'import' | 'export'
}

export interface CreateConsignmentResponse {
  consignmentId: string
}