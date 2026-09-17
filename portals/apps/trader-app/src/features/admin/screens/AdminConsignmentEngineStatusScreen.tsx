import { useCallback, useEffect, useRef, useState } from 'react'
import { Link, useParams } from 'react-router-dom'
import { Badge, Button, Dialog, IconButton, Spinner, Text, Tooltip } from '@radix-ui/themes'
import {
  ChevronRightIcon,
  DoubleArrowDownIcon,
  DoubleArrowUpIcon,
  EyeNoneIcon,
  EyeOpenIcon,
  ReloadIcon,
} from '@radix-ui/react-icons'
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

// START/END carry no ops-actionable signal of their own — whether a workflow reached END is
// already visible from its own status badge (COMPLETED), and a START simply means "this
// execution began," true of every non-empty node list. Hidden by default at every nesting level
// (root, child branches, task workflows alike) to cut noise; the eye toggle in the header shows
// them all when actually needed (e.g. checking a START/END's own timestamp).
const NOISY_NODE_TYPES = new Set(['START', 'END'])

function visibleNodes(nodes: EngineNode[], showAllNodes: boolean): EngineNode[] {
  return showAllNodes ? nodes : nodes.filter((node) => !NOISY_NODE_TYPES.has(node.type))
}

// A one-shot "expand everything" / "collapse everything" command from the toolbar, threaded down
// to every useExpandableWorkflow instance. gen only ever increases, so each click is always
// distinguishable from the last even if it repeats the same `expand` value (e.g. clicking
// "Expand all" again after manually collapsing one branch must still force it back open — a
// plain boolean wouldn't re-trigger since React bails out on setting state to its current value).
// Branches lazily fetch on first expand, so a branch revealed only after its parent's fetch
// completes must also pick up the current signal on mount, not just the ones that existed at
// click time — see useExpandableWorkflow's effect.
interface ExpandSignal {
  gen: number
  expand: boolean
}

const NO_EXPAND_SIGNAL: ExpandSignal = { gen: 0, expand: false }

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

  const topLevelNodes = visibleNodes(status.nodes, showAllNodes)

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
                showAllNodes={showAllNodes}
                expandSignal={expandSignal}
                onOpenVariables={setVariablesTarget}
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
function useExpandableWorkflow(workflowId: string, kind: WorkflowBranchKind, expandSignal: ExpandSignal) {
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

  return { expanded, toggle, loading, status, error }
}

// A node's last_error, collapsed to one truncated line by default (most errors are noise you
// just need to confirm exists) with a small toggle to expand it in place — wrapped, full-width,
// plain selectable text — rather than a popover, so highlighting and copying the message doesn't
// fight a floating layer that can dismiss mid-selection.
function ErrorCell({ message }: { message: string }) {
  const [expanded, setExpanded] = useState(false)
  return (
    <div className="flex items-start gap-1">
      <button
        type="button"
        onClick={() => setExpanded((prev) => !prev)}
        title={expanded ? 'Collapse error' : 'Expand error'}
        className="shrink-0 mt-0.5 text-foreground-muted hover:text-foreground"
      >
        <ChevronRightIcon
          className={`transition-transform ${expanded ? 'rotate-90' : ''}`}
          width={14}
          height={14}
          aria-hidden
        />
      </button>
      {expanded ? (
        <span className="whitespace-pre-wrap break-all">{message}</span>
      ) : (
        <span className="min-w-0 truncate">{message}</span>
      )}
    </div>
  )
}

// One engine node's row. A native engine child (SPLIT_TASK/BATCH_SPLIT/PARALLEL_SPLIT) gets a
// full-width expandable branch below, since those are rare and structurally significant. A TASK
// node's own task workflow (task_workflow_id) is common — nearly every TASK node that has
// started has one — so it gets a small inline toggle in the row instead of a line of its own,
// only expanding into the fuller view below on demand.
function NodeRow({
  node,
  depth,
  showAllNodes,
  expandSignal,
  onOpenVariables,
}: {
  node: EngineNode
  depth: number
  showAllNodes: boolean
  expandSignal: ExpandSignal
  onOpenVariables: (target: WorkflowVariablesTarget) => void
}) {
  const childIds = node.child_workflow_ids ?? []
  const taskBranch = useExpandableWorkflow(node.task_workflow_id ?? '', 'task', expandSignal)

  // Indentation lives only on the Node cell's own padding, never on the grid row/container
  // itself — the grid's own column tracks (Type/Status/Updated/Last error) are fixed-width, so
  // padding on the row would push every one of them right by the same amount, throwing nested
  // rows out of sync with the header and with sibling rows at a different depth.
  return (
    <div className="border-b border-app-border last:border-0">
      <div className={`${NODE_ROW_GRID} py-2`}>
        <div className="pr-3 font-mono text-xs truncate" style={{ paddingLeft: 12 + depth * 20 }} title={node.id}>
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
            <ChevronRightIcon
              className={`shrink-0 transition-transform ${node.task_workflow_id ? 'text-foreground' : 'invisible'} ${
                taskBranch.expanded ? 'rotate-90' : ''
              }`}
              width={16}
              height={16}
              aria-hidden
            />
          </button>
        </div>
        <div className="px-3">
          <Badge color={NODE_STATUS_COLOR[node.status]}>{node.status}</Badge>
        </div>
        <div className="px-3 text-xs text-foreground-muted">{formatDateTime(node.updated_at)}</div>
        <div className="px-3 text-xs text-red-600 min-w-0">
          {node.last_error && <ErrorCell message={node.last_error} />}
        </div>
      </div>
      {childIds.map((childId) => (
        <ChildWorkflowBranch
          key={childId}
          workflowId={childId}
          depth={depth + 1}
          showAllNodes={showAllNodes}
          expandSignal={expandSignal}
          onOpenVariables={onOpenVariables}
        />
      ))}
      {node.task_workflow_id && taskBranch.expanded && (
        <TaskWorkflowPanel
          workflowId={node.task_workflow_id}
          depth={depth + 1}
          loading={taskBranch.loading}
          status={taskBranch.status}
          error={taskBranch.error}
          showAllNodes={showAllNodes}
          expandSignal={expandSignal}
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
  showAllNodes,
  expandSignal,
  onOpenVariables,
}: {
  workflowId: string
  depth: number
  showAllNodes: boolean
  expandSignal: ExpandSignal
  onOpenVariables: (target: WorkflowVariablesTarget) => void
}) {
  const { expanded, toggle, loading, status, error } = useExpandableWorkflow(workflowId, 'child', expandSignal)

  return (
    <>
      {/* Tinted, left-railed header marks this as belonging to a different workflow instance
          from its parent — otherwise it's easy to mistake a child workflow's nodes for more of
          the parent's own list. Indented via its own margin, not a wrapper around the body
          below: NodeRow's grid rows must never sit inside a padded/margined ancestor, or their
          fixed-width Type/Status/Updated columns drift out of sync with the header and with
          sibling rows at a different depth. */}
      <div className="my-1 bg-primary-subtle border-l-2 border-primary rounded" style={{ marginLeft: depth * 20 }}>
        <div className="flex items-center justify-between pr-3">
          <button
            type="button"
            onClick={toggle}
            className="flex items-center gap-1.5 py-1.5 px-3 text-xs font-mono text-foreground-muted hover:text-foreground text-left"
          >
            <ChevronRightIcon className={`shrink-0 transition-transform ${expanded ? 'rotate-90' : ''}`} aria-hidden />
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
      </div>

      {expanded && (
        <WorkflowBranchBody
          depth={depth}
          loading={loading}
          status={status}
          error={error}
          showAllNodes={showAllNodes}
          expandSignal={expandSignal}
          onOpenVariables={onOpenVariables}
        />
      )}
    </>
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
  showAllNodes,
  expandSignal,
  onOpenVariables,
}: {
  workflowId: string
  depth: number
  loading: boolean
  status: EngineStatus | null
  error: FetchError
  showAllNodes: boolean
  expandSignal: ExpandSignal
  onOpenVariables: (target: WorkflowVariablesTarget) => void
}) {
  // The header is indented via its own margin, not a wrapper around the body below — see
  // ChildWorkflowBranch's comment on why NodeRow's grid rows must never sit inside a
  // padded/margined ancestor.
  return (
    <>
      <div
        className="border-l-2 border-app-border flex items-center justify-between pr-3 py-1 pl-2"
        style={{ marginLeft: 12 + depth * 20 }}
      >
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
        showAllNodes={showAllNodes}
        expandSignal={expandSignal}
        onOpenVariables={onOpenVariables}
      />
    </>
  )
}

// The loading/error/node-list body shared by ChildWorkflowBranch and TaskWorkflowPanel once
// expanded.
function WorkflowBranchBody({
  depth,
  loading,
  status,
  error,
  showAllNodes,
  expandSignal,
  onOpenVariables,
}: {
  depth: number
  loading: boolean
  status: EngineStatus | null
  error: FetchError
  showAllNodes: boolean
  expandSignal: ExpandSignal
  onOpenVariables: (target: WorkflowVariablesTarget) => void
}) {
  const nodes = status ? visibleNodes(status.nodes, showAllNodes) : []
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
        (nodes.length === 0 ? (
          <Text size="1" color="gray" className="block py-1 px-3" style={{ paddingLeft: 20 }}>
            No active or completed nodes yet.
          </Text>
        ) : (
          nodes.map((node) => (
            <NodeRow
              key={node.id}
              node={node}
              depth={depth + 1}
              showAllNodes={showAllNodes}
              expandSignal={expandSignal}
              onOpenVariables={onOpenVariables}
            />
          ))
        ))}
    </div>
  )
}
