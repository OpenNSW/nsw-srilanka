import { useCallback, useEffect, useState } from 'react'
import { Badge, Button, IconButton, Spinner, Text, Tooltip } from '@radix-ui/themes'
import { DoubleArrowDownIcon, DoubleArrowUpIcon, EyeNoneIcon, EyeOpenIcon, ReloadIcon } from '@radix-ui/react-icons'
import { getWorkflowEngineStatus } from '@/features/admin/service'
import type { EngineStatus, EngineWorkflowStatus } from '@/features/admin/types'
import { NodeRow } from '@/features/admin/components/engine/NodeRow'
import {
  NODE_ROW_GRID,
  visibleNodes,
  type ExpandSignal,
  NO_EXPAND_SIGNAL,
  type FetchError,
  type WorkflowVariablesTarget,
  type AdminResolutionTarget,
} from '@/features/admin/components/engine/shared'
import { ResolveAdminInterventionView } from '@/features/admin/components/resolve/ResolveAdminInterventionView'
import { GlobalVariablesButton } from '@/features/admin/components/variables/GlobalVariablesButton'
import { WorkflowVariablesDialog } from '@/features/admin/components/variables/WorkflowVariablesDialog'

const WORKFLOW_STATUS_COLOR: Record<EngineWorkflowStatus, 'orange' | 'green' | 'red'> = {
  RUNNING: 'orange',
  COMPLETED: 'green',
  FAILED: 'red',
}

// One flow's raw engine state: its nodes (child and task workflows expand in place), audit trail
// and variables. onRefresh also reloads the consignment, so the flow list's statuses follow.
export function EngineStatusView({
  workflowId,
  title,
  onRefresh,
}: {
  workflowId: string
  title: string
  onRefresh: () => void
}) {
  const [status, setStatus] = useState<EngineStatus | null>(null)
  const [loading, setLoading] = useState(true)
  const [refreshing, setRefreshing] = useState(false)
  const [error, setError] = useState<FetchError>(null)
  const [variablesTarget, setVariablesTarget] = useState<WorkflowVariablesTarget | null>(null)
  const [resolveTarget, setResolveTarget] = useState<AdminResolutionTarget | null>(null)
  // Applies at every nesting level (root, child branches, task workflows) — see
  // NOISY_NODE_TYPES.
  const [showAllNodes, setShowAllNodes] = useState(false)
  const [expandSignal, setExpandSignal] = useState<ExpandSignal>(NO_EXPAND_SIGNAL)
  // What the next click does — starts at "expand" and flips every click. With branches free to
  // expand/collapse individually, this can't track the true state of every branch, but it
  // doesn't need to: from any partially-expanded state, one click reliably drives everything to
  // the shown direction, and a second click (now flipped) drives it the other way.
  const toggleAllExpansion = useCallback(() => {
    setExpandSignal((prev) => ({ gen: prev.gen + 1, expand: !prev.expand }))
  }, [])

  const refresh = useCallback(async () => {
    setRefreshing(true)
    try {
      const result = await getWorkflowEngineStatus(workflowId)
      setStatus(result)
      setError(result ? null : 'notFound')
    } catch (err) {
      console.error('Failed to fetch workflow engine status:', err)
      setError('loadFailed')
    } finally {
      setRefreshing(false)
    }
    onRefresh()
  }, [workflowId, onRefresh])

  useEffect(() => {
    let cancelled = false
    getWorkflowEngineStatus(workflowId)
      .then((result) => {
        if (cancelled) return
        setStatus(result)
        setError(result ? null : 'notFound')
      })
      .catch((err: unknown) => {
        if (cancelled) return
        console.error('Failed to fetch workflow engine status:', err)
        setError('loadFailed')
      })
      .finally(() => {
        if (!cancelled) setLoading(false)
      })
    return () => {
      cancelled = true
    }
  }, [workflowId])

  if (loading) {
    return (
      <div className="flex items-center justify-center py-12">
        <Spinner size="3" />
        <Text size="3" color="gray" className="ml-3">
          Loading engine status…
        </Text>
      </div>
    )
  }

  if (error || !status) {
    return (
      <div className="bg-app-surface rounded-lg shadow p-8 text-center">
        <Text size="5" color="red" weight="medium" className="block mb-2">
          {error === 'notFound' ? 'No workflow execution found' : 'Failed to load engine status'}
        </Text>
        <Button variant="soft" onClick={() => void refresh()}>
          Try again
        </Button>
      </div>
    )
  }

  // Resolving a node takes over the flow's view rather than opening a dialog — there's enough
  // detail here (last error, cached task result, variables patch) that a modal crowds it. Back and
  // Cancel both just clear resolveTarget, returning to this same debugger view underneath.
  if (resolveTarget) {
    return <ResolveAdminInterventionView target={resolveTarget} onBack={() => setResolveTarget(null)} />
  }

  const topLevelNodes = visibleNodes(status.nodes, showAllNodes)

  return (
    <div>
      <div className="mb-1 flex items-center justify-between">
        <div className="flex items-center gap-3">
          <h2 className="text-lg font-semibold text-foreground">{title}</h2>
          <Badge size="2" color={WORKFLOW_STATUS_COLOR[status.status]} title="Engine execution status">
            {status.status}
          </Badge>
        </div>
        <div className="flex items-center gap-2">
          <GlobalVariablesButton
            workflowId={workflowId}
            label={title}
            variables={status.global_variables}
            onOpen={setVariablesTarget}
          />
          <Button variant="soft" color="blue" size="2" onClick={() => void refresh()} disabled={refreshing}>
            <ReloadIcon className={refreshing ? 'animate-spin' : ''} />
            Refresh
          </Button>
          <Tooltip content={expandSignal.expand ? 'Collapse all' : 'Expand all'}>
            <IconButton
              variant="ghost"
              color="gray"
              size="2"
              onClick={toggleAllExpansion}
              aria-label={expandSignal.expand ? 'Collapse all' : 'Expand all'}
            >
              {expandSignal.expand ? <DoubleArrowUpIcon /> : <DoubleArrowDownIcon />}
            </IconButton>
          </Tooltip>
          <Tooltip content={showAllNodes ? 'Hide START/END nodes' : 'Show START/END nodes'}>
            <IconButton
              variant="ghost"
              color="gray"
              size="2"
              onClick={() => setShowAllNodes((prev) => !prev)}
              aria-label={showAllNodes ? 'Hide START/END nodes' : 'Show START/END nodes'}
            >
              {showAllNodes ? <EyeOpenIcon /> : <EyeNoneIcon />}
            </IconButton>
          </Tooltip>
        </div>
      </div>

      <p className="mb-4 text-xs font-mono text-foreground-muted">{status.workflow_id}</p>

      <div className="bg-app-surface rounded-lg shadow mb-6 overflow-x-auto">
        <div className={`${NODE_ROW_GRID} min-w-[700px] border-b border-border text-left`}>
          <div className="p-3 font-medium text-foreground-muted text-sm">Node</div>
          <div className="p-3 font-medium text-foreground-muted text-sm">Type</div>
          <div className="p-3 font-medium text-foreground-muted text-sm">Status</div>
          <div className="p-3 font-medium text-foreground-muted text-sm">Updated</div>
          <div className="p-3 font-medium text-foreground-muted text-sm">Last error</div>
        </div>
        <div className="min-w-[700px]">
          {topLevelNodes.length === 0 ? (
            <Text size="2" color="gray" className="block p-3">
              No active or completed nodes yet.
            </Text>
          ) : (
            topLevelNodes.map((node) => (
              <NodeRow
                key={node.id}
                node={node}
                depth={0}
                workflowId={workflowId}
                workflowKind="workflow"
                edges={status.edges ?? []}
                variables={status.global_variables}
                showAllNodes={showAllNodes}
                expandSignal={expandSignal}
                onOpenVariables={setVariablesTarget}
                onOpenResolve={setResolveTarget}
                onRefresh={() => void refresh()}
              />
            ))
          )}
        </div>
      </div>

      <div className="bg-app-surface rounded-lg shadow p-4">
        <h2 className="text-sm font-semibold text-foreground mb-2">Audit trail</h2>
        {status.audit_trail.length === 0 ? (
          <Text size="2" color="gray">
            No audit events yet.
          </Text>
        ) : (
          <ul className="text-xs font-mono text-foreground-muted space-y-1">
            {status.audit_trail.map((line, i) => (
              // Audit trail lines have no stable id; index is fine — this list is
              // append-only and re-rendered wholesale on every fetch.
              <li key={i}>{line}</li>
            ))}
          </ul>
        )}
      </div>

      <WorkflowVariablesDialog target={variablesTarget} onClose={() => setVariablesTarget(null)} />
    </div>
  )
}
