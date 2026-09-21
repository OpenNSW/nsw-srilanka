import { http, HttpError } from '@/services/http'
import { API_BASE_URL } from '@/constants'
import type { AdminResolutionRequest, AdminWorkflowKind, EngineStatus } from './types'
import type { ConsignmentDetail } from '@/features/consignment/types'

// Ids are interpolated into URL paths through encodeURIComponent throughout this file. Workflow and
// node ids are "<name>:<uuid>" composites, and the name comes from a workflow definition, so it is
// data, not something this app controls: a "/", "?" or "#" in it would otherwise change the path,
// query or fragment before the server saw the id. The server decodes them (r.PathValue), so an
// encoded ":" (%3A) reaches the handler as the same id as before.

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
  return fetchOrNull(`${API_BASE_URL}/api/v1/admin/consignments/${encodeURIComponent(consignmentId)}/engine-status`)
}

// A TASK node's independent per-task ("micro") workflow (see EngineNode.task_workflow_id) — a
// separate ID space/manager from getConsignmentEngineStatus's consignment/child-workflow IDs.
export function getTaskWorkflowEngineStatus(taskWorkflowId: string): Promise<EngineStatus | null> {
  return fetchOrNull(`${API_BASE_URL}/api/v1/admin/task/${encodeURIComponent(taskWorkflowId)}/engine-status`)
}

// Resolves a node AWAITING_ADMIN. workflowId is the workflow instance containing the node, and
// workflowKind says which route family it belongs to: 'consignment' for the root or a child branch
// (child_workflow_ids), 'task' for a task workflow (task_workflow_id), which is a separate ID space
// with its own route, like the two engine-status functions above. Required rather than defaulted, so
// a caller can't silently address a task workflow through the consignment route. Requires
// nsw:consignment:adminwrite.
export async function resolveAdminIntervention(
  workflowId: string,
  nodeId: string,
  request: AdminResolutionRequest,
  workflowKind: AdminWorkflowKind,
): Promise<void> {
  const route = workflowKind === 'task' ? 'task' : 'consignments'
  await http.request({
    url: `${API_BASE_URL}/api/v1/admin/${route}/${encodeURIComponent(workflowId)}/nodes/${encodeURIComponent(nodeId)}/resolve`,
    method: 'POST',
    data: request,
    attachToken: true,
  })
}

// Ops/admin view of the full consignment detail, no trader/CHA ownership check — not the
// trader/CHA-facing getConsignment() in features/consignment/service.ts, which 404s/403s for
// admins inspecting a consignment outside their own company.
export function getConsignmentForAdmin(consignmentId: string): Promise<ConsignmentDetail | null> {
  return fetchOrNull(`${API_BASE_URL}/api/v1/admin/consignments/${encodeURIComponent(consignmentId)}`)
}
