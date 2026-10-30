import { useCallback, useEffect, useRef, useState } from 'react'
import { getTaskWorkflowEngineStatus, getWorkflowEngineStatus } from '@/features/admin/service'
import type { EngineStatus } from '@/features/admin/types'
import type { ExpandSignal, FetchError } from '@/features/admin/components/engine/shared'

// Which nested-workflow drilldown a branch renders: a native engine child (SPLIT_TASK/
// BATCH_SPLIT/PARALLEL_SPLIT, fetched via getWorkflowEngineStatus) or a TASK node's own task
// workflow (a separate ID space/manager, fetched via getTaskWorkflowEngineStatus).
type WorkflowBranchKind = 'child' | 'task'

const BRANCH_FETCHER: Record<WorkflowBranchKind, (id: string) => Promise<EngineStatus | null>> = {
  child: getWorkflowEngineStatus,
  task: getTaskWorkflowEngineStatus,
}

// Fetch-on-first-expand state shared by the "child workflow" full-width branch and the compact
// inline "task workflow" toggle — same lifecycle, different presentation (see NodeRow/
// ChildWorkflowBranch). Exposes both a cache-respecting ensureFetched (used by toggle/expand-all)
// and a cache-bypassing refetch, the latter used to refresh this specific instance's rows after
// an admin resolves a node inside it.
export function useExpandableWorkflow(workflowId: string, kind: WorkflowBranchKind, expandSignal: ExpandSignal) {
  const [expanded, setExpanded] = useState(false)
  const [fetched, setFetched] = useState(false)
  const [loading, setLoading] = useState(false)
  const [status, setStatus] = useState<EngineStatus | null>(null)
  const [error, setError] = useState<FetchError>(null)

  const refetch = useCallback(() => {
    setLoading(true)
    BRANCH_FETCHER[kind](workflowId)
      .then((result) => {
        setStatus(result)
        setError(result ? null : 'notFound')
        setFetched(true)
      })
      .catch((err: unknown) => {
        console.error(`Failed to fetch ${kind} workflow status:`, err)
        setError('loadFailed')
      })
      .finally(() => {
        setLoading(false)
      })
  }, [workflowId, kind])

  const ensureFetched = useCallback(() => {
    // Most nodes have no task workflow (START/END/GATEWAY/SPLIT_TASK, or a TASK node that hasn't
    // started yet) and are passed in as '' — see NodeRow's `node.task_workflow_id ?? ''`. The
    // toggle button is disabled for those, but "Expand all" drives every mounted instance via
    // expandSignal regardless, so without this check it would fetch an empty-id URL per node.
    if (!workflowId || fetched || loading) return
    refetch()
  }, [workflowId, fetched, loading, refetch])

  const toggle = useCallback(() => {
    setExpanded((prev) => {
      const next = !prev
      if (next) ensureFetched()
      return next
    })
  }, [ensureFetched])

  // ensureFetched's identity changes whenever fetched/loading do; reading the latest version via
  // a ref (kept fresh in an effect, never assigned during render) lets the effect below key off
  // expandSignal alone, without re-firing on every fetch-state change of its own making.
  const ensureFetchedRef = useRef(ensureFetched)
  useEffect(() => {
    ensureFetchedRef.current = ensureFetched
  })

  // Adjusts `expanded` during render in response to a new expand-all/collapse-all signal — the
  // recommended pattern for "reset/adjust state when a prop changes" (react.dev), rather than in
  // an effect: remounting via key (the other common fix for this) would destroy the fetched/
  // status cache this hook exists to keep. Comparing against the last *responded* generation,
  // not the previous render's signal, means a branch mounted mid-"expand all" still picks up the
  // current signal on its very first render.
  const [respondedGen, setRespondedGen] = useState(0)
  if (expandSignal.gen !== respondedGen) {
    setRespondedGen(expandSignal.gen)
    setExpanded(expandSignal.expand)
  }

  // The fetch itself is a real side effect (starts a network request) and so, unlike the state
  // adjustment above, must stay in an effect rather than run directly during render — otherwise
  // React re-invoking the render function (StrictMode, an interrupted render) would fire it
  // again. ensureFetched's own fetched/loading guard makes this idempotent regardless.
  useEffect(() => {
    if (expandSignal.gen !== 0 && expandSignal.expand) ensureFetchedRef.current()
  }, [expandSignal])

  return { expanded, toggle, loading, status, error, refetch }
}
