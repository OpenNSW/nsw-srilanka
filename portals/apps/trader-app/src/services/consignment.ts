import type {
  Consignment,
  ConsignmentListResult,
  CreateConsignmentRequest,
  CreateConsignmentResponse,
  ConsignmentState,
  TradeFlow,
  CHA,
} from './types/consignment'
import { defaultApiClient, type ApiClient } from './api'

export async function createConsignment(
  request: CreateConsignmentRequest,
  apiClient: ApiClient = defaultApiClient
): Promise<CreateConsignmentResponse> {
  return apiClient.post<CreateConsignmentRequest, CreateConsignmentResponse>(
    '/consignments',
    request
  )
}

export async function initializeConsignment(
  consignmentId: string,
  hsCodeIds: string[],
  apiClient: ApiClient = defaultApiClient
): Promise<CreateConsignmentResponse> {
  return apiClient.put<{ hsCodeIds: string[] }, CreateConsignmentResponse>(
    `/consignments/${consignmentId}`,
    { hsCodeIds }
  )
}

export async function getConsignment(
  id: string,
  apiClient: ApiClient = defaultApiClient
): Promise<Consignment | null> {
  try {
    return await apiClient.get<Consignment>(`/consignments/${id}`)
  } catch (error) {
    // Return null for 404s, rethrow other errors
    if (error instanceof Error && error.message.includes('404')) {
      return null
    }
    throw error
  }
}

export async function getCHAs(
  apiClient: ApiClient = defaultApiClient
): Promise<CHA[]> {
  return apiClient.get<CHA[]>('/chas')
}

export async function getAllConsignments(
  offset: number = 0,
  limit: number = 50,
  state?: ConsignmentState | 'all',
  flow?: TradeFlow | 'all',
  role: 'trader' | 'cha' = 'trader',
  apiClient: ApiClient = defaultApiClient
): Promise<ConsignmentListResult> {
  const params: Record<string, string | number> = { offset, limit }
  if (state && state !== 'all') params.state = state
  if (flow && flow !== 'all') params.flow = flow
  params.role = role

  const response = await apiClient.get<ConsignmentListResult>(
    '/consignments',
    params
  )

  return response
}