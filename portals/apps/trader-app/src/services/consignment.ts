import type {
  Consignment,
  ConsignmentListResult,
  CreateConsignmentResponse,
  ConsignmentState,
  TradeFlow,
} from './types/consignment'
import { http } from './http'
import { API_BASE_URL, API_PATH_PREFIX } from '../constants'

const BASE = `${API_BASE_URL}${API_PATH_PREFIX}`

export async function createConsignment(): Promise<CreateConsignmentResponse> {
  const { data } = await http.request({
    url: `${BASE}/consignments`,
    method: 'POST',
    data: {},
    attachToken: true,
  })
  return data as CreateConsignmentResponse
}

export async function getConsignment(id: string): Promise<Consignment | null> {
  try {
    const { data } = await http.request({
      url: `${BASE}/consignments/${id}`,
      attachToken: true,
    })
    return data as Consignment
  } catch (error) {
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
): Promise<ConsignmentListResult> {
  const params: Record<string, string | number> = { offset, limit }
  if (state && state !== 'all') params.state = state
  if (flow && flow !== 'all') params.flow = flow
  params.role = role

  const { data } = await http.request({
    url: `${BASE}/consignments`,
    params,
    attachToken: true,
  })
  return data as ConsignmentListResult
}
