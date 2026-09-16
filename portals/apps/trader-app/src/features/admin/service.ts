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

// A TASK node's independent per-task ("micro") workflow (see EngineNode.task_workflow_id) — a
// separate ID space/manager from getConsignmentEngineStatus's consignment/child-workflow IDs.
export async function getTaskWorkflowEngineStatus(taskWorkflowId: string): Promise<EngineStatus | null> {
  try {
    const { data } = await http.request<EngineStatus>({
      url: `${API_BASE_URL}/api/v1/admin/task-workflows/${taskWorkflowId}/engine-status`,
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

// Ops/admin view of the full consignment detail, no trader/CHA ownership check — not the
// trader/CHA-facing getConsignment() in features/consignment/service.ts, which 404s/403s for
// admins inspecting a consignment outside their own company.
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
