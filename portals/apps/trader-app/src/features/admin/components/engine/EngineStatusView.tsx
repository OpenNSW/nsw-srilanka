import { useCallback, useState } from 'react'
import { IconButton, Text, Tooltip } from '@radix-ui/themes'
import { PageState, RefreshButton, StatusBadge, type StatusTone } from '@portals/ui'
import { DoubleArrowDownIcon, DoubleArrowUpIcon, EyeNoneIcon, EyeOpenIcon } from '@radix-ui/react-icons'
import { useEngineStatus } from '@/features/admin/hooks'
import type { EngineWorkflowStatus } from '@/features/admin/types'
import { NodeRow } from '@/features/admin/components/engine/NodeRow'
import {
  NODE_ROW_GRID,
  visibleNodes,
  type ExpandSignal,
  NO_EXPAND_SIGNAL,
  type WorkflowVariablesTarget,
  type AdminResolutionTarget,
} from '@/features/admin/components/engine/shared'
import { ResolveAdminInterventionView } from '@/features/admin/components/resolve/ResolveAdminInterventionView'
import { GlobalVariablesButton } from '@/features/admin/components/variables/GlobalVariablesButton'
import { WorkflowVariablesDialog } from '@/features/admin/components/variables/WorkflowVariablesDialog'
import { useCan } from '@/hooks/useCan'

const WORKFLOW_STATUS_TONE: Record<EngineWorkflowStatus, StatusTone> = {
  RUNNING: 'info',
  COMPLETED: 'success',
  FAILED: 'danger',
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
  const { status, error, loading, isFetching, refetch } = useEngineStatus(workflowId, 'workflow')
  const [variablesTarget, setVariablesTarget] = useState<WorkflowVariablesTarget | null>(null)
  const [resolveTarget, setResolveTarget] = useState<AdminResolutionTarget | null>(null)
  const canIntervene = useCan('admin.intervene')
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

  const refresh = () => {
    refetch()
    onRefresh()
  }

  if (loading) {
    return <PageState variant="loading" title="Loading engine status…" />
  }

  if (error || !status) {
    return (
      <div className="bg-app-surface rounded-lg shadow">
        <PageState
          variant="error"
          title={error === 'notFound' ? 'No workflow execution found' : 'Failed to load engine status'}
          onRetry={refresh}
        />
      </div>
    )
  }

  // Resolving a node takes over the flow's view rather than opening a dialog — there's enough
  // detail here (last error, cached task result, variables patch) that a modal crowds it. Back and
  // Cancel both just clear resolveTarget, returning to this same debugger view underneath. A user
  // who can no longer intervene is shown the debugger view instead.
  if (resolveTarget && canIntervene) {
    return <ResolveAdminInterventionView target={resolveTarget} onBack={() => setResolveTarget(null)} />
  }

  const topLevelNodes = visibleNodes(status.nodes, showAllNodes)

  return (
    <div>
      <div className="mb-1 flex flex-wrap items-center justify-between gap-2">
        <div className="flex flex-wrap items-center gap-3">
          <h2 className="text-lg font-semibold text-foreground">{title}</h2>
          <StatusBadge tone={WORKFLOW_STATUS_TONE[status.status]} size="2" title="Engine execution status">
            {status.status}
          </StatusBadge>
        </div>
        <div className="flex flex-wrap items-center gap-2">
          <GlobalVariablesButton
            workflowId={workflowId}
            label={title}
            variables={status.global_variables}
            onOpen={setVariablesTarget}
          />
          <RefreshButton label="Refresh" refreshing={isFetching} onClick={refresh} showLabel />
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
                onRefresh={refresh}
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
          <ul className="text-xs font-mono text-foreground-muted space-y-1 break-all">
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
