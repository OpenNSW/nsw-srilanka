import { http, HttpError } from '@/services/http'
import { API_BASE_URL } from '@/constants'
import type { EngineStatus } from './types'
import type { ConsignmentDetail } from '@/features/consignment/types'

// Every admin lookup here treats a 404 as "not found" (null) rather than an error to throw —
// the id was well-formed but nothing (yet, or any more) exists for it, not a broken request.
async function fetchOrNull<T>(url: string): Promise<T | null> {
  try {
    const { data } = await http.request<T>({ url, attachToken: true })
    return data
  } catch (error) {
    if (error instanceof HttpError && error.status === 404) {
      return null
    }
    throw error
  }
}

export function getConsignmentEngineStatus(consignmentId: string): Promise<EngineStatus | null> {
  return fetchOrNull(`${API_BASE_URL}/api/v1/admin/consignments/${consignmentId}/engine-status`)
}

// A TASK node's independent per-task ("micro") workflow (see EngineNode.task_workflow_id) — a
// separate ID space/manager from getConsignmentEngineStatus's consignment/child-workflow IDs.
export function getTaskWorkflowEngineStatus(taskWorkflowId: string): Promise<EngineStatus | null> {
  return fetchOrNull(`${API_BASE_URL}/api/v1/admin/task/${taskWorkflowId}/engine-status`)
}

// Ops/admin view of the full consignment detail, no trader/CHA ownership check — not the
// trader/CHA-facing getConsignment() in features/consignment/service.ts, which 404s/403s for
// admins inspecting a consignment outside their own company.
export function getConsignmentForAdmin(consignmentId: string): Promise<ConsignmentDetail | null> {
  return fetchOrNull(`${API_BASE_URL}/api/v1/admin/consignments/${consignmentId}`)
}
