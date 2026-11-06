import { useMutation } from '@/hooks/useMutation'
import { useQuery, type UseQueryResult } from '@/hooks/useQuery'
import { isAgencyMode } from '@/features/case/mode'
import type { FetchError } from '@/features/admin/components/engine/shared'
import {
  forceParkNode,
  getCaseForAdmin,
  getConsignmentForAdmin,
  getTaskWorkflowEngineStatus,
  getWorkflowEngineStatus,
  resolveAdminIntervention,
} from '@/features/admin/service'
import type { AdminConsignmentDetail, AdminWorkflowKind, EngineStatus } from '@/features/admin/types'

// getConsignmentForAdmin in TNSW, getCaseForAdmin in an agency: the same AdminConsignmentDetail
// shape either way (see getCaseForAdmin), minus the trade-only fields.
const fetchAdminDetail = isAgencyMode ? getCaseForAdmin : getConsignmentForAdmin

// A flow and a child branch are both read through the workflows route; a task workflow is a
// separate ID space with its own route.
const ENGINE_STATUS_FETCHER: Record<
  AdminWorkflowKind,
  (workflowId: string, signal: AbortSignal) => Promise<EngineStatus | null>
> = {
  workflow: getWorkflowEngineStatus,
  task: getTaskWorkflowEngineStatus,
}

// The admin fetchers resolve to null on a 404: null is notFound, a rejection is loadFailed.
function fetchErrorOf<T>(query: UseQueryResult<T | null>): FetchError {
  if (query.status === 'error') return 'loadFailed'
  if (query.status === 'success' && query.data === null) return 'notFound'
  return null
}

export interface UseAdminConsignmentResult {
  consignment: AdminConsignmentDetail | null
  error: FetchError
  // No result yet: the first load, or a retry after a failure.
  loading: boolean
  refetch: () => void
}

// The consignment (or an agency's case) the debugger shows. A new ID drops the old one's data.
export function useAdminConsignment(consignmentId: string | undefined): UseAdminConsignmentResult {
  const query = useQuery(consignmentId ? ['admin-consignment', consignmentId] : null, (signal) =>
    fetchAdminDetail(consignmentId!, signal),
  )

  return {
    consignment: query.data ?? null,
    error: fetchErrorOf(query),
    loading: query.status === 'pending',
    refetch: () => void query.refetch(),
  }
}

export interface UseEngineStatusOptions {
  enabled?: boolean
  // Part of the query key: a new value reloads from scratch.
  attempt?: number
}

export interface UseEngineStatusResult {
  status: EngineStatus | null
  error: FetchError
  // No result yet: the first load, or a retry after a failure.
  loading: boolean
  // Any load in flight, including a refresh.
  isFetching: boolean
  refetch: () => void
}

// One workflow instance's raw engine state. An empty workflowId never loads.
export function useEngineStatus(
  workflowId: string,
  kind: AdminWorkflowKind,
  { enabled = true, attempt = 0 }: UseEngineStatusOptions = {},
): UseEngineStatusResult {
  const query = useQuery(
    workflowId ? ['admin-engine-status', kind, workflowId, attempt] : null,
    (signal) => ENGINE_STATUS_FETCHER[kind](workflowId, signal),
    { enabled },
  )

  return {
    status: query.data ?? null,
    error: fetchErrorOf(query),
    loading: query.status === 'pending',
    isFetching: query.isFetching,
    refetch: () => void query.refetch(),
  }
}

// See forceParkNode. mutate rejects on failure; error keeps it until the next attempt.
export function useForcePark() {
  return useMutation(forceParkNode)
}

// See resolveAdminIntervention. mutate rejects on failure; error keeps it until the next attempt.
export function useResolveAdminIntervention() {
  return useMutation(resolveAdminIntervention)
}
