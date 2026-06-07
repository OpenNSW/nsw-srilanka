import type {
  Consignment,
  ConsignmentListResult,
  CreateConsignmentResponse,
  ConsignmentState,
  TradeFlow,
} from './types/consignment'
import { defaultApiClient, type ApiClient } from './api'

// startConsignment creates an export consignment and starts its workflow directly — no CHA
// company or HS code is collected up front; the workflow's own tasks (CHA selection, then
// HS code selection) collect those later.
export async function startConsignment(apiClient: ApiClient = defaultApiClient): Promise<CreateConsignmentResponse> {
  return apiClient.post<Record<string, never>, CreateConsignmentResponse>('/consignments/start', {})
}

export async function getConsignment(id: string, apiClient: ApiClient = defaultApiClient): Promise<Consignment | null> {
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

export async function getAllConsignments(
  offset: number = 0,
  limit: number = 50,
  state?: ConsignmentState | 'all',
  flow?: TradeFlow | 'all',
  role: 'trader' | 'cha' = 'trader',
  apiClient: ApiClient = defaultApiClient,
): Promise<ConsignmentListResult> {
  const params: Record<string, string | number> = { offset, limit }
  if (state && state !== 'all') params.state = state
  if (flow && flow !== 'all') params.flow = flow
  params.role = role

  const response = await apiClient.get<ConsignmentListResult>('/consignments', params)

  return response
}
