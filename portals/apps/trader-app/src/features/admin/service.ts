import { http, HttpError } from '@/services/http'
import { API_BASE_URL } from '@/constants'
import type { EngineStatus } from './types'
import type { ConsignmentDetail } from '@/features/consignment/types'

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

// Ops/admin view of the full consignment detail — GET /api/v1/admin/consignments/{id}, gated on
// ConsignmentAdminRead with no trader/CHA ownership check (see HandleAdminGetConsignmentByID on
// the backend). Deliberately not the trader/CHA-facing getConsignment() in
// features/consignment/service.ts, which would 404/403 for the admins this screen serves.
export async function getConsignmentForAdmin(consignmentId: string): Promise<ConsignmentDetail | null> {
  try {
    const { data } = await http.request<ConsignmentDetail>({
      url: `${API_BASE_URL}/api/v1/admin/consignments/${consignmentId}`,
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
