import { http, HttpError } from '@/services/http'
import { API_BASE_URL } from '@/constants'
import type { EngineStatus } from './types'

export async function getConsignmentEngineStatus(consignmentId: string): Promise<EngineStatus | null> {
  try {
    const { data } = await http.request<EngineStatus>({
      url: `${API_BASE_URL}/api/v1/admin/consignments/${consignmentId}/engine-status`,
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
