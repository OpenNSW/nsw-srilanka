import { apiPost, apiGet, USE_MOCK } from './api'
import type {
  Consignment,
  CreateConsignmentRequest,
  CreateConsignmentResponse,
} from './types/consignment'

// In-memory store for mock consignments
const mockConsignments: Map<string, Consignment> = new Map()

// Initialize with some sample data
const sampleConsignments: Consignment[] = [
  {
    id: 'CON-001',
    hsCode: '09021011',
    hsCodeDescription: 'Certified by Sri Lanka Tea Board as wholly of Sri Lanka origin, flavoured (≤4g packing)',
    workflowId: 'wf-09021011-export',
    workflowName: 'Tea Export Permit - Sri Lanka Certified Flavoured',
    workflowType: 'export',
    status: 'completed',
    currentStepId: 'EndEvent_1',
    createdAt: '2024-01-15T10:30:00Z',
    updatedAt: '2024-01-18T14:20:00Z',
  },
  {
    id: 'CON-002',
    hsCode: '09023011',
    hsCodeDescription: 'Certified by Sri Lanka Tea Board as wholly of Sri Lanka origin, flavoured (≤4g packing)',
    workflowId: 'wf-09023011-import',
    workflowName: 'Tea Import Permit - Sri Lanka Certified Flavoured',
    workflowType: 'import',
    status: 'in_progress',
    currentStepId: 'Task_2',
    createdAt: '2024-01-16T09:15:00Z',
    updatedAt: '2024-01-17T11:45:00Z',
  },
  {
    id: 'CON-003',
    hsCode: '09022019',
    hsCodeDescription: 'Other (3kg-5kg packing)',
    workflowId: 'wf-09022019-export',
    workflowName: 'Tea Export Permit - Other',
    workflowType: 'export',
    status: 'pending',
    currentStepId: 'Task_1',
    createdAt: '2024-01-17T14:00:00Z',
    updatedAt: '2024-01-17T14:00:00Z',
  },
  {
    id: 'CON-004',
    hsCode: '09024091',
    hsCodeDescription: 'Certified by Sri Lanka Tea Board as wholly of Sri Lanka origin, flavoured (bulk)',
    workflowId: 'wf-09024091-import',
    workflowName: 'Tea Import Permit - Sri Lanka Certified Flavoured',
    workflowType: 'import',
    status: 'rejected',
    currentStepId: 'EndEvent_2',
    createdAt: '2024-01-10T08:30:00Z',
    updatedAt: '2024-01-12T16:00:00Z',
  },
  {
    id: 'CON-005',
    hsCode: '09021031',
    hsCodeDescription: 'Certified by Sri Lanka Tea Board as wholly of Sri Lanka origin, flavoured (1kg-3kg packing)',
    workflowId: 'wf-09021031-export',
    workflowName: 'Tea Export Permit - Sri Lanka Certified Flavoured',
    workflowType: 'export',
    status: 'completed',
    currentStepId: 'EndEvent_1',
    createdAt: '2024-01-05T11:20:00Z',
    updatedAt: '2024-01-08T09:30:00Z',
  },
]

// Initialize mock data
sampleConsignments.forEach((c) => mockConsignments.set(c.id, c))

function generateConsignmentId(): string {
  const timestamp = Date.now().toString(36)
  const random = Math.random().toString(36).substring(2, 8)
  return `CON-${timestamp}-${random}`.toUpperCase()
}

async function mockCreateConsignment(
  request: CreateConsignmentRequest
): Promise<CreateConsignmentResponse> {
  const consignmentId = generateConsignmentId()
  const now = new Date().toISOString()

  const consignment: Consignment = {
    id: consignmentId,
    hsCode: request.hsCode,
    hsCodeDescription: request.hsCodeDescription,
    workflowId: request.workflowId,
    workflowName: request.workflowName,
    workflowType: request.workflowType,
    status: 'pending',
    currentStepId: 'StartEvent_1',
    createdAt: now,
    updatedAt: now,
  }

  mockConsignments.set(consignmentId, consignment)

  return { consignmentId }
}

async function mockGetConsignment(id: string): Promise<Consignment | null> {
  return mockConsignments.get(id) || null
}

export async function createConsignment(
  request: CreateConsignmentRequest
): Promise<CreateConsignmentResponse> {
  if (USE_MOCK) {
    // Simulate network delay
    await new Promise((resolve) => setTimeout(resolve, 500))
    return mockCreateConsignment(request)
  }

  return apiPost<CreateConsignmentRequest, CreateConsignmentResponse>(
    '/consignments',
    request
  )
}

export async function getConsignment(id: string): Promise<Consignment | null> {
  if (USE_MOCK) {
    // Simulate network delay
    await new Promise((resolve) => setTimeout(resolve, 200))
    return mockGetConsignment(id)
  }

  return apiGet<Consignment>(`/consignments/${id}`)
}

export async function getAllConsignments(): Promise<Consignment[]> {
  if (USE_MOCK) {
    // Simulate network delay
    await new Promise((resolve) => setTimeout(resolve, 300))
    // Return consignments sorted by createdAt (newest first)
    return Array.from(mockConsignments.values()).sort(
      (a, b) => new Date(b.createdAt).getTime() - new Date(a.createdAt).getTime()
    )
  }

  return apiGet<Consignment[]>('/consignments')
}