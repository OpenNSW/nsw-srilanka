import { useCallback, useEffect, useState } from 'react'
import { Link, useParams } from 'react-router-dom'
import { Badge, Button, Dialog, Spinner, Text } from '@radix-ui/themes'
import { ReloadIcon } from '@radix-ui/react-icons'
import {
  getConsignmentEngineStatus,
  getConsignmentForAdmin,
  getTaskWorkflowEngineStatus,
} from '@/features/admin/service'
import type { EngineNode, EngineNodeStatus, EngineStatus, EngineWorkflowStatus } from '@/features/admin/types'
import type { ConsignmentDetail } from '@/features/consignment/types'
import { formatDateTime, formatState, getStateColor } from '@/features/consignment/utils'

const WORKFLOW_STATUS_COLOR: Record<EngineWorkflowStatus, 'orange' | 'green' | 'red'> = {
  RUNNING: 'orange',
  COMPLETED: 'green',
  FAILED: 'red',
}

const NODE_STATUS_COLOR: Record<EngineNodeStatus, 'gray' | 'orange' | 'green' | 'red' | 'amber'> = {
  NOT_STARTED: 'gray',
  RUNNING: 'orange',
  COMPLETED: 'green',
  FAILED: 'red',
  AWAITING_ADMIN: 'amber',
}

// Shared grid so NodeRow's columns line up under the header regardless of nesting depth.
const NODE_ROW_GRID = 'grid grid-cols-[1fr_110px_130px_150px_1fr] gap-2 items-center'

// A fetch either found the workflow, didn't (404 — normal for a not-yet-started or already-gone
// execution), or failed for some other reason.
type FetchError = 'notFound' | 'loadFailed' | null

// Internal ops view of a consignment's raw engine state — the same picture you'd
// otherwise need the Temporal UI for. Backend gates this behind the
// ConsignmentAdminRead scope (see HandleGetConsignmentEngineStatus); reachable
// only by direct URL, not linked from trader/CHA navigation.
export function AdminConsignmentEngineStatusScreen() {
  const { consignmentId } = useParams<{ consignmentId: string }>()

  if (!consignmentId) {
    return (
      <div className="p-6">
        <Text color="red">A consignment ID is required.</Text>
      </div>
    )
  }

  // Keyed on consignmentId so navigating to a different root remounts fresh.
  return <EngineStatusView key={consignmentId} workflowId={consignmentId} />
}

// Identifies which workflow instance's global variables are open in the dialog — the root
// consignment workflow, or one of its expanded child branches. Each workflow instance has its
// own independent WorkflowVariables snapshot, so the dialog is always scoped to exactly one.
interface WorkflowVariablesTarget {
  workflowId: string
  label: string
  variables?: Record<string, unknown>
}

function EngineStatusView({ workflowId }: { workflowId: string }) {
  const [status, setStatus] = useState<EngineStatus | null>(null)
  const [loading, setLoading] = useState(true)
  const [refreshing, setRefreshing] = useState(false)
  const [error, setError] = useState<FetchError>(null)
  const [variablesTarget, setVariablesTarget] = useState<WorkflowVariablesTarget | null>(null)
  // Supplementary business-side context (name, state, trader) fetched independently of the
  // engine status — best-effort only, so a failure here just hides this section rather than
  // blocking the ops view the admin actually came here for.
  const [consignment, setConsignment] = useState<ConsignmentDetail | null>(null)

  useEffect(() => {
    let cancelled = false
    getConsignmentForAdmin(workflowId)
      .then((result) => {
        if (!cancelled) setConsignment(result)
      })
      .catch((err: unknown) => {
        console.error('Failed to fetch consignment details:', err)
      })
    return () => {
      cancelled = true
    }
  }, [workflowId])

  const refresh = useCallback(async () => {
    setRefreshing(true)
    try {
      const result = await getConsignmentEngineStatus(workflowId)
      setStatus(result)
      setError(result ? null : 'notFound')
    } catch (err) {
      console.error('Failed to fetch consignment engine status:', err)
      setError('loadFailed')
    } finally {
      setRefreshing(false)
    }
    // Best-effort: the business-side panel just keeps its last-known values on failure.
    getConsignmentForAdmin(workflowId)
      .then(setConsignment)
      .catch((err: unknown) => console.error('Failed to fetch consignment details:', err))
  }, [workflowId])

  useEffect(() => {
    let cancelled = false
    getConsignmentEngineStatus(workflowId)
      .then((result) => {
        if (cancelled) return
        setStatus(result)
        setError(result ? null : 'notFound')
      })
      .catch((err: unknown) => {
        if (cancelled) return
        console.error('Failed to fetch consignment engine status:', err)
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
      <div className="p-6 flex items-center justify-center py-12">
        <Spinner size="3" />
        <Text size="3" color="gray" className="ml-3">
          Loading engine status…
        </Text>
      </div>
    )
  }

  if (error || !status) {
    return (
      <div className="p-6">
        <div className="bg-app-surface rounded-lg shadow p-8 text-center">
          <Text size="5" color="red" weight="medium" className="block mb-2">
            {error === 'notFound' ? 'No workflow execution found' : 'Failed to load engine status'}
          </Text>
          <Button variant="soft" onClick={() => void refresh()}>
            Try again
          </Button>
        </div>
      </div>
    )
  }

  return (
    <div className="p-4 md:p-6">
      <div className="mb-1 flex items-center justify-between">
        <div className="flex items-center gap-3">
          <h1 className="text-xl font-semibold text-foreground">Consignment Debugger</h1>
          <Badge size="2" color={WORKFLOW_STATUS_COLOR[status.status]} title="Engine execution status">
            {status.status}
          </Badge>
        </div>
        <div className="flex items-center gap-2">
          <Button variant="soft" color="gray" size="2" asChild>
            <Link to={`/admin/consignments/${workflowId}/view`}>View consignment</Link>
          </Button>
          <GlobalVariablesButton
            workflowId={workflowId}
            label="Root workflow"
            variables={status.global_variables}
            onOpen={setVariablesTarget}
          />
          <Button variant="soft" color="blue" size="2" onClick={() => void refresh()} disabled={refreshing}>
            <ReloadIcon className={refreshing ? 'animate-spin' : ''} />
            Refresh
          </Button>
        </div>
      </div>

      {consignment && (
        <div className="mb-2 flex items-center gap-2 flex-wrap">
          <Text size="3" weight="medium" className="text-foreground">
            {consignment.name || 'Untitled consignment'}
          </Text>
          <Badge size="1" color={getStateColor(consignment.state)}>
            {formatState(consignment.state)}
          </Badge>
          <Badge size="1" variant="soft" color={consignment.flow === 'IMPORT' ? 'blue' : 'green'}>
            {consignment.flow}
          </Badge>
        </div>
      )}

      <div className="mb-6 flex items-center gap-4 flex-wrap">
        <p className="text-xs font-mono text-foreground-muted">{status.consignment_id}</p>
        {consignment && (
          <>
            <p className="text-xs text-foreground-muted">Created {formatDateTime(consignment.createdAt)}</p>
            <p className="text-xs text-foreground-muted">
              Trader <span className="font-mono">{consignment.traderId}</span>
            </p>
          </>
        )}
      </div>

      <div className="bg-app-surface rounded-lg shadow mb-6 overflow-x-auto">
        <div className={`${NODE_ROW_GRID} min-w-[700px] border-b border-app-border text-left`}>
          <div className="p-3 font-medium text-foreground-subtle text-sm">Node</div>
          <div className="p-3 font-medium text-foreground-subtle text-sm">Type</div>
          <div className="p-3 font-medium text-foreground-subtle text-sm">Status</div>
          <div className="p-3 font-medium text-foreground-subtle text-sm">Updated</div>
          <div className="p-3 font-medium text-foreground-subtle text-sm">Last error</div>
        </div>
        <div className="min-w-[700px]">
          {status.nodes.length === 0 ? (
            <Text size="2" color="gray" className="block p-3">
              No active or completed nodes yet.
            </Text>
          ) : (
            status.nodes.map((node) => (
              <NodeRow key={node.id} node={node} depth={0} onOpenVariables={setVariablesTarget} />
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

// Global variables belong to a workflow instance (root or child), not to any one node — opened
// from a "Global variables" action per workflow instance rather than per node row, which would
// misleadingly imply the data is node-specific.
function WorkflowVariablesDialog({ target, onClose }: { target: WorkflowVariablesTarget | null; onClose: () => void }) {
  return (
    <Dialog.Root open={target !== null} onOpenChange={(open) => !open && onClose()}>
      <Dialog.Content maxWidth="600px">
        {target && (
          <>
            <Dialog.Title>{target.label} global variables</Dialog.Title>
            <Dialog.Description size="2" color="gray" className="font-mono break-all mb-4">
              {target.workflowId}
            </Dialog.Description>

            {target.variables && Object.keys(target.variables).length > 0 ? (
              <pre className="bg-app-surface-muted rounded p-3 text-xs font-mono overflow-auto max-h-96 whitespace-pre-wrap break-all">
                {JSON.stringify(target.variables, null, 2)}
              </pre>
            ) : (
              <Text size="2" color="gray">
                No global variables recorded for this workflow.
              </Text>
            )}

            <div className="flex justify-end mt-4">
              <Dialog.Close>
                <Button variant="soft" color="gray">
                  Close
                </Button>
              </Dialog.Close>
            </div>
          </>
        )}
      </Dialog.Content>
    </Dialog.Root>
  )
}

// The "Global variables" action that opens WorkflowVariablesDialog scoped to one workflow
// instance — used at the root header (a full-size toolbar button) and by each nested branch
// (a smaller inline one), which differ only in size/variant and which workflow/label they open.
function GlobalVariablesButton({
  workflowId,
  label,
  variables,
  onOpen,
  size = '2',
  variant = 'soft',
}: {
  workflowId: string
  label: string
  variables?: Record<string, unknown>
  onOpen: (target: WorkflowVariablesTarget) => void
  size?: '1' | '2'
  variant?: 'soft' | 'ghost'
}) {
  return (
    <Button variant={variant} color="gray" size={size} onClick={() => onOpen({ workflowId, label, variables })}>
      Global variables
    </Button>
  )
}

// Which nested-workflow drilldown a branch renders: a native engine child (SPLIT_TASK/
// BATCH_SPLIT/PARALLEL_SPLIT, fetched via getConsignmentEngineStatus) or a TASK node's own task
// workflow (a separate ID space/manager, fetched via getTaskWorkflowEngineStatus).
type WorkflowBranchKind = 'child' | 'task'

const BRANCH_FETCHER: Record<WorkflowBranchKind, (id: string) => Promise<EngineStatus | null>> = {
  child: getConsignmentEngineStatus,
  task: getTaskWorkflowEngineStatus,
}

// Fetch-on-first-expand state shared by the "child workflow" full-width branch and the compact
// inline "task workflow" toggle — same lifecycle, different presentation (see NodeRow/
// ChildWorkflowBranch).
function useExpandableWorkflow(workflowId: string, kind: WorkflowBranchKind) {
  const [expanded, setExpanded] = useState(false)
  const [fetched, setFetched] = useState(false)
  const [loading, setLoading] = useState(false)
  const [status, setStatus] = useState<EngineStatus | null>(null)
  const [error, setError] = useState<FetchError>(null)

  const ensureFetched = useCallback(() => {
    if (fetched || loading) return
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
  }, [workflowId, kind, fetched, loading])

  const toggle = useCallback(() => {
    setExpanded((prev) => {
      const next = !prev
      if (next) ensureFetched()
      return next
    })
  }, [ensureFetched])

  return { expanded, toggle, loading, status, error }
}

// One engine node's row. A native engine child (SPLIT_TASK/BATCH_SPLIT/PARALLEL_SPLIT) gets a
// full-width expandable branch below, since those are rare and structurally significant. A TASK
// node's own task workflow (task_workflow_id) is common — nearly every TASK node that has
// started has one — so it gets a small inline toggle in the row instead of a line of its own,
// only expanding into the fuller view below on demand.
function NodeRow({
  node,
  depth,
  onOpenVariables,
}: {
  node: EngineNode
  depth: number
  onOpenVariables: (target: WorkflowVariablesTarget) => void
}) {
  const childIds = node.child_workflow_ids ?? []
  const taskBranch = useExpandableWorkflow(node.task_workflow_id ?? '', 'task')

  return (
    <div className="border-b border-app-border last:border-0">
      <div className={`${NODE_ROW_GRID} py-2`} style={{ paddingLeft: depth * 20 }}>
        <div className="px-3 font-mono text-xs truncate" title={node.id}>
          {node.id}
        </div>
        <div className="px-3 text-sm">
          {/* Always the same button, at the same size, whether or not this node has a task
              workflow — just disabled/inert (and the arrow hidden) when it doesn't, so every
              row's height stays identical instead of nodes with one shifting layout relative to
              nodes without. The whole label + arrow is the tap target, not just the arrow, and
              -mx-1/-my-1 grow the hit area past the visible padding without nudging the text. */}
          <button
            type="button"
            onClick={taskBranch.toggle}
            disabled={!node.task_workflow_id}
            title={node.task_workflow_id ? 'View task workflow' : undefined}
            className={`flex items-center gap-1 w-full -mx-1 -my-1 px-1 py-1 rounded text-left ${
              node.task_workflow_id ? 'hover:bg-app-surface-muted cursor-pointer' : 'cursor-default'
            }`}
          >
            <span className="truncate">{node.gateway_type ?? node.type}</span>
            <span
              className={`inline-block text-base leading-none shrink-0 transition-transform ${
                node.task_workflow_id ? '' : 'invisible'
              } ${taskBranch.expanded ? 'rotate-90' : ''}`}
              aria-hidden
            >
              ▸
            </span>
          </button>
        </div>
        <div className="px-3">
          <Badge color={NODE_STATUS_COLOR[node.status]}>{node.status}</Badge>
        </div>
        <div className="px-3 text-xs text-foreground-muted">{formatDateTime(node.updated_at)}</div>
        <div className="px-3 text-xs text-red-600 truncate" title={node.last_error}>
          {node.last_error ?? ''}
        </div>
      </div>
      {childIds.map((childId) => (
        <ChildWorkflowBranch key={childId} workflowId={childId} depth={depth + 1} onOpenVariables={onOpenVariables} />
      ))}
      {node.task_workflow_id && taskBranch.expanded && (
        <TaskWorkflowPanel
          workflowId={node.task_workflow_id}
          depth={depth + 1}
          loading={taskBranch.loading}
          status={taskBranch.status}
          error={taskBranch.error}
          onOpenVariables={onOpenVariables}
        />
      )}
    </div>
  )
}

// A collapsed-by-default full-width row for one native engine child workflow (SPLIT_TASK/
// BATCH_SPLIT/PARALLEL_SPLIT). Fetches its engine status only on first expand, and keeps the
// result cached in local state so collapsing and re-expanding doesn't re-fetch. Once fetched,
// exposes a "Global variables" action scoped to this specific workflow instance.
function ChildWorkflowBranch({
  workflowId,
  depth,
  onOpenVariables,
}: {
  workflowId: string
  depth: number
  onOpenVariables: (target: WorkflowVariablesTarget) => void
}) {
  const { expanded, toggle, loading, status, error } = useExpandableWorkflow(workflowId, 'child')

  return (
    <div style={{ paddingLeft: depth * 20 }}>
      {/* Tinted, left-railed container marks this whole subtree as belonging to a different
          workflow instance from its parent — otherwise it's easy to mistake a child workflow's
          nodes for more of the parent's own list, especially once nested a few levels deep. */}
      <div className="my-1 bg-primary-subtle border-l-2 border-primary rounded">
        <div className="flex items-center justify-between pr-3">
          <button
            type="button"
            onClick={toggle}
            className="flex items-center gap-1.5 py-1.5 px-3 text-xs font-mono text-foreground-muted hover:text-foreground text-left"
          >
            <span
              className={`inline-block text-[10px] transition-transform ${expanded ? 'rotate-90' : ''}`}
              aria-hidden
            >
              ▸
            </span>
            <span className="text-foreground-subtle">child workflow</span>
            {workflowId}
          </button>
          {status && (
            <GlobalVariablesButton
              workflowId={workflowId}
              label="Child workflow"
              variables={status.global_variables}
              onOpen={onOpenVariables}
              size="1"
              variant="ghost"
            />
          )}
        </div>

        {expanded && (
          <WorkflowBranchBody
            depth={depth}
            loading={loading}
            status={status}
            error={error}
            onOpenVariables={onOpenVariables}
          />
        )}
      </div>
    </div>
  )
}

// The expanded contents of a task workflow toggled open from NodeRow's inline affordance — a
// lighter-weight, un-boxed counterpart to ChildWorkflowBranch's tinted card, since a task
// workflow is expected on nearly every TASK node rather than being an occasional structural
// branch. Still gets its own "Global variables" action and ID, just without the full-line
// always-visible toggle bar.
function TaskWorkflowPanel({
  workflowId,
  depth,
  loading,
  status,
  error,
  onOpenVariables,
}: {
  workflowId: string
  depth: number
  loading: boolean
  status: EngineStatus | null
  error: FetchError
  onOpenVariables: (target: WorkflowVariablesTarget) => void
}) {
  return (
    <div className="border-l-2 border-app-border ml-3" style={{ paddingLeft: depth * 20 }}>
      <div className="flex items-center justify-between pr-3 py-1 pl-2">
        <span className="text-[11px] font-mono text-foreground-muted truncate" title={workflowId}>
          <span className="text-foreground-subtle">task workflow </span>
          {workflowId}
        </span>
        {status && (
          <GlobalVariablesButton
            workflowId={workflowId}
            label="Task workflow"
            variables={status.global_variables}
            onOpen={onOpenVariables}
            size="1"
            variant="ghost"
          />
        )}
      </div>
      <WorkflowBranchBody
        depth={depth}
        loading={loading}
        status={status}
        error={error}
        onOpenVariables={onOpenVariables}
      />
    </div>
  )
}

// The loading/error/node-list body shared by ChildWorkflowBranch and TaskWorkflowPanel once
// expanded.
function WorkflowBranchBody({
  depth,
  loading,
  status,
  error,
  onOpenVariables,
}: {
  depth: number
  loading: boolean
  status: EngineStatus | null
  error: FetchError
  onOpenVariables: (target: WorkflowVariablesTarget) => void
}) {
  return (
    <div className="pb-1">
      {loading && (
        <div className="flex items-center gap-2 py-1 px-3" style={{ paddingLeft: 20 }}>
          <Spinner size="1" />
          <Text size="1" color="gray">
            Loading…
          </Text>
        </div>
      )}
      {error && (
        <Text size="1" color="red" className="block py-1 px-3" style={{ paddingLeft: 20 }}>
          {error === 'notFound' ? 'No workflow execution found' : 'Failed to load'}
        </Text>
      )}
      {status &&
        (status.nodes.length === 0 ? (
          <Text size="1" color="gray" className="block py-1 px-3" style={{ paddingLeft: 20 }}>
            No active or completed nodes yet.
          </Text>
        ) : (
          status.nodes.map((node) => (
            <NodeRow key={node.id} node={node} depth={depth + 1} onOpenVariables={onOpenVariables} />
          ))
        ))}
    </div>
  )
}
