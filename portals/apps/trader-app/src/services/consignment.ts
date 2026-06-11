import type {
  Consignment,
  ConsignmentListResult,
  CreateConsignmentResponse,
  ConsignmentState,
  TradeFlow,
} from './types/consignment'
import { http, HttpError } from './http'
import { API_BASE_URL } from '../constants'

export async function createConsignment(): Promise<CreateConsignmentResponse> {
  const { data } = await http.request<CreateConsignmentResponse>({
    url: `${API_BASE_URL}/api/v1/consignments`,
    method: 'POST',
    data: {},
    attachToken: true,
  })
  return data
}

export async function getConsignment(id: string): Promise<Consignment | null> {
  try {
    const { data } = await http.request<Consignment>({
      url: `${API_BASE_URL}/api/v1/consignments/${id}`,
      attachToken: true,
    })
    return data
  } catch (error) {
    if (error instanceof HttpError && error.status === 404) {
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
  q?: string,
): Promise<ConsignmentListResult> {
  const params: Record<string, string | number> = { offset, limit }
  if (state && state !== 'all') params.state = state
  if (flow && flow !== 'all') params.flow = flow
  params.role = role
  if (q && q.trim() !== '') params.q = q.trim()

  const { data } = await http.request<ConsignmentListResult>({
    url: `${API_BASE_URL}/api/v1/consignments`,
    params,
    attachToken: true,
  })
  return data
}
