import { useCallback, useState } from 'react'
import { useEngineStatus } from '@/features/admin/hooks'
import type { AdminWorkflowKind } from '@/features/admin/types'
import type { ExpandSignal } from '@/features/admin/components/engine/shared'

// Which nested-workflow drilldown a branch renders: a native engine child (SPLIT_TASK/
// BATCH_SPLIT/PARALLEL_SPLIT, read through the workflows route) or a TASK node's own task
// workflow (a separate ID space/manager with its own route).
type WorkflowBranchKind = 'child' | 'task'

const BRANCH_ROUTE: Record<WorkflowBranchKind, AdminWorkflowKind> = {
  child: 'workflow',
  task: 'task',
}

// Fetch-on-first-expand state shared by the "child workflow" full-width branch and the compact
// inline "task workflow" toggle — same lifecycle, different presentation (see NodeRow/
// ChildWorkflowBranch). The first expand loads the branch; collapsing and re-expanding keeps the
// result, unless the last load failed, in which case expanding loads it again. refetch reloads
// this specific instance's rows, e.g. after an admin resolves a node inside it.
//
// Most nodes have no task workflow (START/END/GATEWAY/SPLIT_TASK, or a TASK node that hasn't
// started yet) and are passed in as '' — see NodeRow's `node.task_workflow_id ?? ''`. The toggle
// button is disabled for those, and "Expand all", which drives every mounted instance through
// expandSignal, never loads one: useEngineStatus skips an empty ID.
export function useExpandableWorkflow(workflowId: string, kind: WorkflowBranchKind, expandSignal: ExpandSignal) {
  const [expanded, setExpanded] = useState(false)
  // 0 until the first expand; each expand that needs a load bumps it.
  const [attempt, setAttempt] = useState(0)
  const branch = useEngineStatus(workflowId, BRANCH_ROUTE[kind], { enabled: attempt > 0, attempt })

  const toggle = useCallback(() => {
    setExpanded((prev) => !prev)
  }, [])

  // Adjusts `expanded` during render in response to a new expand-all/collapse-all signal — the
  // recommended pattern for "reset/adjust state when a prop changes" (react.dev), rather than in
  // an effect: remounting via key (the other common fix for this) would destroy the cached
  // result this hook exists to keep. Comparing against the last *responded* generation, not the
  // previous render's signal, means a branch mounted mid-"expand all" still picks up the current
  // signal on its very first render.
  const [respondedGen, setRespondedGen] = useState(0)
  if (expandSignal.gen !== respondedGen) {
    setRespondedGen(expandSignal.gen)
    setExpanded(expandSignal.expand)
  }

  // Load on first expand, and again on expand after a failed load (not after notFound).
  const [respondedExpanded, setRespondedExpanded] = useState(false)
  if (expanded !== respondedExpanded) {
    setRespondedExpanded(expanded)
    if (expanded && (attempt === 0 || branch.error === 'loadFailed')) {
      setAttempt((n) => n + 1)
    }
  }

  return {
    expanded,
    toggle,
    loading: branch.isFetching,
    status: branch.status,
    error: branch.error,
    refetch: branch.refetch,
  }
}
