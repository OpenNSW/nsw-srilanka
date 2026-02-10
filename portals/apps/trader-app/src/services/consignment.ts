import type {
  Consignment,
  CreateConsignmentRequest,
  CreateConsignmentResponse,
} from './types/consignment'
import { apiGet, apiPost } from './api'

// TODO: Get from auth context
const DEFAULT_TRADER_ID = 'TRADER-001'

export async function createConsignment(
  request: CreateConsignmentRequest
): Promise<CreateConsignmentResponse> {
  return apiPost<CreateConsignmentRequest, CreateConsignmentResponse>(
    '/consignments',
    request
  )
}

export async function getConsignment(id: string): Promise<Consignment | null> {
  try {
    return await apiGet<Consignment>(`/consignments/${id}`)
  } catch (error) {
    // Return null for 404s, rethrow other errors
    if (error instanceof Error && error.message.includes('404')) {
      return null
    }
    throw error
  }
}

export async function getAllConsignments(
  traderId: string = DEFAULT_TRADER_ID
): Promise<Consignment[]> {
  return apiGet<Consignment[]>('/consignments', {
    traderId,
  })
}