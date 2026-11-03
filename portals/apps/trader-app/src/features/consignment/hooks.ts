import { useRef, useState } from 'react'
import { useMutation } from '@/hooks/useMutation'
import { useQuery } from '@/hooks/useQuery'
import type { ConsignmentDetail } from '@/features/consignment/types.ts'
import { getConsignment, startConsignmentFlow } from '@/features/consignment/service.ts'

// A freshly created consignment can be returned before its workflow has been provisioned
// (workflowNodes still empty). Rather than flashing the empty "no workflow" state, re-fetch a
// bounded number of times with exponential backoff until the nodes appear or the consignment
// reaches a terminal state. The same backoff waits for a newly started flow's first step.
export const MAX_POLLS = 5
const POLL_BASE_DELAY_MS = 1000
const POLL_MAX_DELAY_MS = 5000

export type ConsignmentErrorKey = 'idRequired' | 'notFound' | 'loadFailed'

type PollReason = 'workflow' | 'newStep'

export interface UseConsignmentOptions {
  // Part of the query key, so views of one consignment through different endpoints (the
  // trader's and the admin's) never share a result.
  scope?: string
  // Defaults to the trader/CHA-facing, ownership-checked getConsignment.
  fetcher?: (consignmentId: string, signal?: AbortSignal) => Promise<ConsignmentDetail | null>
}

export interface UseConsignmentResult {
  consignment: ConsignmentDetail | null
  error: ConsignmentErrorKey | null
  // The first load, before any consignment is shown.
  loading: boolean
  // Waiting for a new consignment's workflow to be provisioned.
  provisioning: boolean
  // Reloading a consignment already shown, including the wait for a started flow's first step.
  refreshing: boolean
  refetch: () => void
  // Starts a flow, then reloads until its first step appears. Rejects if the start fails.
  startFlow: (templateId: string) => Promise<void>
}

export function useConsignment(
  consignmentId: string | undefined,
  { scope = 'trader', fetcher = getConsignment }: UseConsignmentOptions = {},
): UseConsignmentResult {
  // The step IDs shown when a flow was started, or null when no start is pending. Polling
  // continues until a step outside this set (the new flow's first) appears.
  const stepsBeforeStartRef = useRef<Set<string> | null>(null)
  const [pollReason, setPollReason] = useState<PollReason | null>(null)

  const query = useQuery(
    consignmentId ? ['consignment', scope, consignmentId] : null,
    (signal) => fetcher(consignmentId!, signal),
    {
      // Called after every fetch, and with the last result after a failed poll, so a transient
      // failure keeps retrying instead of surfacing until the polls run out.
      refetchInterval: (data, fetchCount) => {
        const reason = data && fetchCount <= MAX_POLLS ? awaiting(data, stepsBeforeStartRef.current) : null
        setPollReason(reason)
        if (reason === null) {
          stepsBeforeStartRef.current = null
          return false
        }
        return Math.min(POLL_BASE_DELAY_MS * 2 ** (fetchCount - 1), POLL_MAX_DELAY_MS)
      },
    },
  )

  const { mutate: startFlow } = useMutation(async (templateId: string) => {
    if (!consignmentId) return
    await startConsignmentFlow(consignmentId, templateId)
    // The started flow's first step is created a moment after the start returns.
    stepsBeforeStartRef.current = new Set((query.data?.workflowNodes ?? []).map((node) => node.id))
    void query.refetch()
  })

  const refetch = () => {
    stepsBeforeStartRef.current = null
    void query.refetch()
  }

  const active = query.isFetching || query.polling
  const consignment = query.data ?? null
  // A started flow is awaited on the current view, under the refresh spinner, not the full-page
  // set-up spinner.
  const provisioning = active && pollReason === 'workflow'

  let error: ConsignmentErrorKey | null = null
  if (!consignmentId) error = 'idRequired'
  else if (query.status === 'error') error = 'loadFailed'
  else if (query.data === null) error = 'notFound'

  return {
    consignment,
    error,
    loading: !!consignmentId && query.status === 'pending',
    provisioning,
    refreshing: active && consignment !== null && !provisioning,
    refetch,
    startFlow,
  }
}

function awaiting(consignment: ConsignmentDetail, stepsBeforeStart: Set<string> | null): PollReason | null {
  const nodes = consignment.workflowNodes ?? []
  if (stepsBeforeStart !== null && nodes.every((node) => stepsBeforeStart.has(node.id))) return 'newStep'
  if (nodes.length === 0 && consignment.state === 'IN_PROGRESS') return 'workflow'
  return null
}
