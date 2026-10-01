import { http, HttpError } from '@/services/http'
import { API_BASE_URL } from '@/constants'
import type { ConsignmentDetail } from '@/features/consignment/types'
import type { CaseListResult } from './types'

export async function getCases(offset: number = 0, limit: number = 50): Promise<CaseListResult> {
  const { data } = await http.request<CaseListResult>({
    url: `${API_BASE_URL}/api/v1/cases`,
    params: { offset, limit },
    attachToken: true,
  })
  return data
}

// Typed as ConsignmentDetail so ConsignmentDetailScreen can take it as its fetcher. The
// backend returns the same field names but none of the trade-only ones (flow, trader,
// items, globalContext), which the screen tolerates being absent.
export async function getCase(id: string): Promise<ConsignmentDetail | null> {
  try {
    const { data } = await http.request<ConsignmentDetail>({
      url: `${API_BASE_URL}/api/v1/cases/${id}`,
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
