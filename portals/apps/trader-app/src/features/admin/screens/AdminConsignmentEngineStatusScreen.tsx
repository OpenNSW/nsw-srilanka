import { Fragment, useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { Link, useParams } from 'react-router-dom'
import { AlertDialog, Badge, Button, Dialog, IconButton, Spinner, Text, TextArea, Tooltip } from '@radix-ui/themes'
import {
  ArrowLeftIcon,
  ChevronRightIcon,
  DoubleArrowDownIcon,
  DoubleArrowUpIcon,
  EyeNoneIcon,
  EyeOpenIcon,
  InfoCircledIcon,
  ReloadIcon,
} from '@radix-ui/react-icons'
import {
  getConsignmentEngineStatus,
  getConsignmentForAdmin,
  getTaskWorkflowEngineStatus,
  resolveAdminIntervention,
} from '@/features/admin/service'
import type {
  AdminResolutionAction,
  EngineNode,
  EngineNodeStatus,
  EngineStatus,
  EngineWorkflowStatus,
} from '@/features/admin/types'
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
const NODE_ROW_GRID = 'grid grid-cols-[1fr_150px_130px_150px_1fr] gap-2 items-center'

// node.type/gateway_type are shouty-snake-case engine identifiers (SPLIT_TASK, PARALLEL_SPLIT,
// EXCLUSIVE_JOIN, ...) — humanized for display rather than shown as-is.
function humanizeNodeType(type: string): string {
  return type
    .toLowerCase()
    .split('_')
    .map((word) => word.charAt(0).toUpperCase() + word.slice(1))
    .join(' ')
}

// Workflow/node ids are "<name>:<uuid>" composites — the name is what an admin actually
// recognizes at a glance; the uuid matters for exact lookups but is unreadable noise inline, so
// it's split off here to be shown smaller/muted with the full id in a title tooltip instead.
function splitIDName(id: string): { name: string; uuid: string | null } {
  const separatorIndex = id.lastIndexOf(':')
  return separatorIndex === -1
    ? { name: id, uuid: null }
    : { name: id.slice(0, separatorIndex), uuid: id.slice(separatorIndex + 1) }
}

// START/END carry no ops-actionable signal of their own — whether a workflow reached END is
// already visible from its own status badge (COMPLETED), and a START simply means "this
// execution began," true of every non-empty node list. Hidden by default at every nesting level
// (root, child branches, task workflows alike) to cut noise; the eye toggle in the header shows
// them all when actually needed (e.g. checking a START/END's own timestamp).
const NOISY_NODE_TYPES = new Set(['START', 'END'])

function visibleNodes(nodes: EngineNode[], showAllNodes: boolean): EngineNode[] {
  return showAllNodes ? nodes : nodes.filter((node) => !NOISY_NODE_TYPES.has(node.type))
}

// A one-shot "expand everything"/"collapse everything" command from the toolbar, threaded down
// to every useExpandableWorkflow instance. gen === 0 means no command has been issued yet — a
// plain boolean can't represent that third state, and without it every branch would eagerly
// expand and fetch on mount before the toggle was ever clicked. Branches lazily fetch on first
// expand, so a branch revealed only after its parent's fetch completes must also pick up the
// current signal on mount, not just the ones that existed at click time — see
// useExpandableWorkflow's effect.
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

// Identifies the node an admin is resolving, plus which workflow instance it belongs to (root,
// a child branch, or a task workflow — see NodeRow) and how to refresh that instance's view once
// resolved. lastError and cachedTaskResult are carried along purely so the resolve view can show
// them without a second fetch — see ResolveAdminInterventionView.
interface AdminResolutionTarget {
  workflowId: string
  nodeId: string
  isGateway: boolean
  lastError?: string
  cachedTaskResult?: Record<string, unknown>
  onResolved: () => void
}

function EngineStatusView({ workflowId }: { workflowId: string }) {
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

  // Resolving a node takes over the whole screen rather than opening a dialog — there's enough
  // detail here (last error, cached task result, overrides JSON) that a modal crowds it. Back and
  // Cancel both just clear resolveTarget, returning to this same debugger view underneath.
  if (resolveTarget) {
    return <ResolveAdminInterventionView target={resolveTarget} onBack={() => setResolveTarget(null)} />
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
                workflowId={workflowId}
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

// Line-level diff between the cached value's pretty-printed JSON and the override draft being
// typed, via the standard LCS/longest-common-subsequence algorithm — so an inserted or deleted
// line shifts the rest of the comparison instead of misaligning everything after it, the way a
// naive index-by-index line comparison would. O(m·n); MAX_DIFF_CELLS below guards against a
// pathological paste making that quadratic cost actually matter.
interface DiffLine {
  type: 'unchanged' | 'added' | 'removed'
  text: string
  // 1-based, matching how an editor would number them — null on the side a line doesn't exist
  // on (an added line has no oldLineNo, a removed line has no newLineNo), same as GitHub's
  // split gutter.
  oldLineNo: number | null
  newLineNo: number | null
}

const MAX_DIFF_CELLS = 200_000

function diffLines(oldText: string, newText: string): DiffLine[] | null {
  const oldLines = oldText.split('\n')
  const newLines = newText.split('\n')
  const m = oldLines.length
  const n = newLines.length
  if (m * n > MAX_DIFF_CELLS) return null

  const lcs: number[][] = Array.from({ length: m + 1 }, () => new Array<number>(n + 1).fill(0))
  for (let i = m - 1; i >= 0; i--) {
    for (let j = n - 1; j >= 0; j--) {
      lcs[i][j] = oldLines[i] === newLines[j] ? lcs[i + 1][j + 1] + 1 : Math.max(lcs[i + 1][j], lcs[i][j + 1])
    }
  }

  const result: DiffLine[] = []
  let i = 0
  let j = 0
  while (i < m && j < n) {
    if (oldLines[i] === newLines[j]) {
      result.push({ type: 'unchanged', text: oldLines[i], oldLineNo: i + 1, newLineNo: j + 1 })
      i++
      j++
    } else if (lcs[i + 1][j] >= lcs[i][j + 1]) {
      result.push({ type: 'removed', text: oldLines[i], oldLineNo: i + 1, newLineNo: null })
      i++
    } else {
      result.push({ type: 'added', text: newLines[j], oldLineNo: null, newLineNo: j + 1 })
      j++
    }
  }
  while (i < m) {
    result.push({ type: 'removed', text: oldLines[i], oldLineNo: i + 1, newLineNo: null })
    i++
  }
  while (j < n) {
    result.push({ type: 'added', text: newLines[j], oldLineNo: null, newLineNo: j + 1 })
    j++
  }
  return result
}

// Only added/removed — unchanged lines are filtered out of the rendered diff entirely (see
// toDiffRows), so there's nothing that ever needs to style or prefix one.
const DIFF_LINE_CLASS: Record<'added' | 'removed', string> = {
  added: 'bg-success-subtle text-success-strong',
  removed: 'bg-error-subtle text-error-strong',
}

const DIFF_LINE_PREFIX: Record<'added' | 'removed', string> = {
  added: '+',
  removed: '−',
}

// Character-level diff, same LCS approach as diffLines but over individual characters instead
// of lines — used only to highlight exactly what changed within a paired removed/added line
// (see toDiffRows' "replace" rows), not for the line-level diff itself. Consecutive same-type
// characters are merged into one segment, so rendering is one <span> per changed/unchanged run
// rather than one per character. A much smaller cell budget than MAX_DIFF_CELLS: this only ever
// runs on a single line's worth of text, not the whole document.
interface CharDiffSegment {
  type: 'unchanged' | 'added' | 'removed'
  text: string
}

const MAX_CHAR_DIFF_CELLS = 20_000

function diffChars(oldStr: string, newStr: string): CharDiffSegment[] | null {
  const oldChars = Array.from(oldStr)
  const newChars = Array.from(newStr)
  const m = oldChars.length
  const n = newChars.length
  if (m * n > MAX_CHAR_DIFF_CELLS) return null

  const lcs: number[][] = Array.from({ length: m + 1 }, () => new Array<number>(n + 1).fill(0))
  for (let i = m - 1; i >= 0; i--) {
    for (let j = n - 1; j >= 0; j--) {
      lcs[i][j] = oldChars[i] === newChars[j] ? lcs[i + 1][j + 1] + 1 : Math.max(lcs[i + 1][j], lcs[i][j + 1])
    }
  }

  const segments: CharDiffSegment[] = []
  const push = (type: CharDiffSegment['type'], char: string) => {
    const last = segments[segments.length - 1]
    if (last && last.type === type) {
      last.text += char
    } else {
      segments.push({ type, text: char })
    }
  }

  let i = 0
  let j = 0
  while (i < m && j < n) {
    if (oldChars[i] === newChars[j]) {
      push('unchanged', oldChars[i])
      i++
      j++
    } else if (lcs[i + 1][j] >= lcs[i][j + 1]) {
      push('removed', oldChars[i])
      i++
    } else {
      push('added', newChars[j])
      j++
    }
  }
  while (i < m) {
    push('removed', oldChars[i])
    i++
  }
  while (j < n) {
    push('added', newChars[j])
    j++
  }
  return segments
}

// A collapsed run of unchanged lines between two visible changes, rendered as a single "⋯ N
// unchanged lines" marker instead of showing every line — the point of hiding unchanged lines at
// all is a concise diff, so a long untouched stretch shouldn't silently re-inflate it back to a
// wall of text. "replace" pairs a removed line with the added line immediately below it (see
// toDiffRows) so the two can be rendered with a character-level highlight instead of two
// unrelated whole-line ones — much clearer for a typo-sized edit like a single field rename.
type DiffRow =
  | { kind: 'line'; line: DiffLine & { type: 'added' | 'removed' } }
  | { kind: 'gap'; count: number }
  | {
      kind: 'replace'
      removed: DiffLine & { type: 'removed' }
      added: DiffLine & { type: 'added' }
      charDiff: CharDiffSegment[] | null
    }

// Reduces a full line-by-line diff to just the rows worth showing. A removed run immediately
// followed by an added run (core's LCS output always orders a "line changed" this way) is a
// stronger signal that those lines replace each other than that they're unrelated deletions and
// insertions, so they're paired 1:1 (shortest run wins; any leftover lines on the longer side
// fall back to plain whole-line rows) and each pair gets a character-level diff to highlight
// exactly what changed. A gap marker fills any stretch of unchanged lines skipped in between.
function toDiffRows(diff: DiffLine[]): DiffRow[] {
  const rows: DiffRow[] = []
  let lastVisible = -1
  let i = 0
  while (i < diff.length) {
    const line = diff[i]
    if (line.type === 'unchanged') {
      i++
      continue
    }
    if (lastVisible !== -1 && i - lastVisible > 1) {
      rows.push({ kind: 'gap', count: i - lastVisible - 1 })
    }

    if (line.type === 'added') {
      // An added run with nothing removed directly before it (a pure insertion) — the removed
      // branch below already consumes any added run that immediately follows a removed one.
      rows.push({ kind: 'line', line: line as DiffLine & { type: 'added' } })
      lastVisible = i
      i++
      continue
    }

    let removedEnd = i
    while (removedEnd < diff.length && diff[removedEnd].type === 'removed') removedEnd++
    let addedEnd = removedEnd
    while (addedEnd < diff.length && diff[addedEnd].type === 'added') addedEnd++

    const removedLines = diff.slice(i, removedEnd) as (DiffLine & { type: 'removed' })[]
    const addedLines = diff.slice(removedEnd, addedEnd) as (DiffLine & { type: 'added' })[]
    const pairCount = Math.min(removedLines.length, addedLines.length)
    for (let k = 0; k < pairCount; k++) {
      const removed = removedLines[k]
      const added = addedLines[k]
      rows.push({ kind: 'replace', removed, added, charDiff: diffChars(removed.text, added.text) })
    }
    for (let k = pairCount; k < removedLines.length; k++) rows.push({ kind: 'line', line: removedLines[k] })
    for (let k = pairCount; k < addedLines.length; k++) rows.push({ kind: 'line', line: addedLines[k] })

    lastVisible = addedEnd - 1
    i = addedEnd
  }
  return rows
}

// Stronger-highlight classes for just the character-level segments that differ within a
// "replace" row's two lines — layered on top of DIFF_LINE_CLASS's whole-line tint, which stays
// as the backdrop for the segments that didn't change. White text on the saturated background
// keeps this readable — text-success-strong/text-error-strong (dark green/red) on top of an
// already-saturated bg-success/bg-error is too little contrast to read comfortably.
const CHAR_DIFF_CLASS: Record<'added' | 'removed', string> = {
  added: 'bg-success text-white rounded-sm',
  removed: 'bg-error text-white rounded-sm',
}

// Renders one side (old or new) of a character-level diff: the unchanged run plain, and only
// that side's own changed run highlighted — a "removed" segment never appears on the added side
// and vice versa, since those characters never existed there.
function renderCharDiff(segments: CharDiffSegment[], side: 'added' | 'removed') {
  return segments
    .filter((segment) => segment.type === 'unchanged' || segment.type === side)
    .map((segment, i) => (
      <span key={i} className={segment.type === side ? CHAR_DIFF_CLASS[side] : undefined}>
        {segment.text}
      </span>
    ))
}

// One row of the diff panel's split gutter — shared by a plain added/removed DiffRow and each
// half of a "replace" pair, so the two only ever differ in what they pass as children (plain
// text vs. a character-highlighted one).
function DiffLineRow({
  oldLineNo,
  newLineNo,
  type,
  children,
}: {
  oldLineNo: number | null
  newLineNo: number | null
  type: 'added' | 'removed'
  children: React.ReactNode
}) {
  return (
    <div className={`grid grid-cols-[2rem_2rem_auto] gap-2 px-1 -mx-1 whitespace-pre ${DIFF_LINE_CLASS[type]}`}>
      <span className="text-right text-foreground-subtle select-none">{oldLineNo ?? ''}</span>
      <span className="text-right text-foreground-subtle select-none">{newLineNo ?? ''}</span>
      <span>
        <span className="select-none mr-1 opacity-70">{DIFF_LINE_PREFIX[type]}</span>
        {children}
      </span>
    </div>
  )
}

// A plain <textarea> with a synced line-number gutter, standing in for Radix's TextArea only in
// the Overrides editor (see ResolveAdminInterventionView) — Radix's own component doesn't expose
// the scroll position a gutter needs to stay in sync. Wrapping is deliberately off (wrap="off" +
// white-space: pre + horizontal scroll) rather than left to wrap: with it on, a long line's
// wrapped continuation would visually sit under whichever number happens to be next, since a
// gutter line only ever corresponds to one real line — the same reason the read-only diff panel
// next to this editor doesn't wrap either.
function LineNumberedTextArea({ value, onChange }: { value: string; onChange: (value: string) => void }) {
  const textareaRef = useRef<HTMLTextAreaElement>(null)
  const gutterRef = useRef<HTMLDivElement>(null)
  const lineCount = value.split('\n').length

  const syncGutterScroll = () => {
    if (textareaRef.current && gutterRef.current) {
      gutterRef.current.scrollTop = textareaRef.current.scrollTop
    }
  }

  return (
    // The border lives on this wrapper, not the <textarea> — a bare <textarea> carries its own
    // UA-default border independent of any wrapper border, which without an explicit border-0
    // shows through as a second, darker (often black) border nested just inside this one.
    <div className="flex border border-app-border rounded overflow-hidden font-mono text-xs h-72 focus-within:border-primary">
      <div ref={gutterRef} className="shrink-0 w-9 overflow-hidden bg-app-surface-muted text-right py-2 pr-2 text-foreground-subtle select-none" aria-hidden>
        {Array.from({ length: lineCount }, (_, i) => (
          <div key={i} className="leading-5">
            {i + 1}
          </div>
        ))}
      </div>
      <textarea
        ref={textareaRef}
        value={value}
        onChange={(e) => onChange(e.target.value)}
        onScroll={syncGutterScroll}
        spellCheck={false}
        wrap="off"
        className="flex-1 min-w-0 resize-none border-0 p-2 leading-5 outline-none whitespace-pre overflow-auto bg-transparent"
      />
    </div>
  )
}

// Which resolution actions exist and whether core's engine rejects them for a GATEWAY node (a
// gateway's routing can't be skipped/overridden without bypassing its own condition logic — see
// core/workflow.parkNodeForAdmin). description is shown via an info icon next to each button —
// see the ADMIN_ACTIONS.map below.
const ADMIN_ACTIONS: {
  action: AdminResolutionAction
  label: string
  color: 'blue' | 'green' | 'gray' | 'red'
  disabledForGateway: boolean
  description: string
}[] = [
  {
    action: 'RETRY',
    label: 'Retry',
    color: 'blue',
    disabledForGateway: false,
    description:
      "Re-runs the node for real — re-calls the Activity for a TASK node, or re-evaluates the routing condition for a GATEWAY. Override Inputs (if provided below) are merged into workflow variables first and used as input to that run, not as the result — e.g. to correct a variable a GATEWAY's condition depends on before it re-evaluates.",
  },
  {
    action: 'OVERRIDE',
    label: 'Override',
    color: 'green',
    disabledForGateway: true,
    description:
      "Skips re-running anything — merges the Override Outputs below directly into workflow variables as if they were the node's final output, then marks it completed. Unavailable for GATEWAY nodes: completing this way always takes the first outgoing edge, which would silently ignore a gateway's actual routing condition.",
  },
  {
    action: 'SKIP',
    label: 'Skip',
    color: 'gray',
    disabledForGateway: true,
    description:
      "Marks the node completed without setting any workflow variables, then continues down its first outgoing edge. Unavailable for GATEWAY nodes, for the same reason as Override — it can't decide the correct edge without evaluating the condition.",
  },
  {
    action: 'ABORT',
    label: 'Abort',
    color: 'red',
    disabledForGateway: false,
    description: "Fails this node and the whole workflow with the node's original error. Use when the workflow genuinely can't continue.",
  },
]

// Lets an admin resolve one AWAITING_ADMIN node (see NodeRow's "Resolve" button) by picking one
// of RETRY/OVERRIDE/SKIP/ABORT, a required reason, and optional JSON overrides. Takes over the
// whole screen (see EngineStatusView) rather than a dialog — last error, cached task result, and
// overrides JSON add up to more than a modal comfortably holds. Calls target.onResolved() on
// success so the caller can refresh just the affected instance's view, then onBack() — same as
// Back/Cancel, which both just return to the debugger view underneath without resolving anything.
function ResolveAdminInterventionView({ target, onBack }: { target: AdminResolutionTarget; onBack: () => void }) {
  const [action, setAction] = useState<AdminResolutionAction | null>(null)
  const [reason, setReason] = useState('')
  // Pretty-printed cached value, or '{}' when there's nothing cached — the fixed baseline the
  // diff panel below always compares the edit box against. Same formatting on both sides of the
  // diff (see overridesText's initial value) is what makes the line-level comparison meaningful
  // rather than showing every line as changed over an indentation mismatch.
  const originalPretty = target.cachedTaskResult ? JSON.stringify(target.cachedTaskResult, null, 2) : '{}'
  // Starts pre-filled with the current cached value (or '{}') so an admin edits from the real
  // data instead of blank JSON — see the "Reset" button to get back here after editing.
  const [overridesText, setOverridesText] = useState(originalPretty)
  const [submitting, setSubmitting] = useState(false)
  const [error, setError] = useState<string | null>(null)
  // Gates the actual submit behind an explicit re-confirmation — this mutates a live workflow
  // (possibly irreversibly for ABORT) and there's no undo, so "Submit" opens this dialog instead
  // of calling submit() directly.
  const [confirmOpen, setConfirmOpen] = useState(false)

  // Only used to warn under the edit box — the diff panel itself works on raw text and tolerates
  // mid-edit invalid JSON fine, but a final invalid JSON would fail on submit.
  const overridesJSONError = useMemo(() => {
    try {
      JSON.parse(overridesText)
      return null
    } catch {
      return 'Not valid JSON yet.'
    }
  }, [overridesText])

  const overridesDiff = useMemo(() => diffLines(originalPretty, overridesText), [originalPretty, overridesText])
  const overridesDiffRows = useMemo(() => (overridesDiff ? toDiffRows(overridesDiff) : null), [overridesDiff])

  const submit = async () => {
    if (!action) return
    // Only OVERRIDE/RETRY actually consume Overrides (see the ADMIN_ACTIONS descriptions above)
    // — for SKIP/ABORT, overridesText is just its unshown default (the cached result, or '{}'),
    // not anything the admin chose to send, so it must not go out on the wire.
    let overrides: Record<string, unknown> | undefined
    if ((action === 'OVERRIDE' || action === 'RETRY') && overridesText.trim()) {
      try {
        overrides = JSON.parse(overridesText) as Record<string, unknown>
      } catch {
        setConfirmOpen(false)
        setError('Overrides must be valid JSON.')
        return
      }
    }
    setConfirmOpen(false)
    setSubmitting(true)
    setError(null)
    try {
      await resolveAdminIntervention(target.workflowId, target.nodeId, { action, overrides, reason })
      target.onResolved()
      onBack()
    } catch (err) {
      console.error('Failed to resolve admin intervention:', err)
      setError(err instanceof Error ? err.message : 'Failed to resolve admin intervention.')
    } finally {
      setSubmitting(false)
    }
  }

  const resetOverridesToCachedValue = () => setOverridesText(originalPretty)

  return (
    <div className="p-4 md:p-6">
      <div className="mb-4">
        <Button variant="ghost" color="gray" onClick={onBack} disabled={submitting}>
          <ArrowLeftIcon />
          Back
        </Button>
      </div>

      <h1 className="text-xl font-semibold text-foreground mb-2">Resolve admin intervention</h1>
      <div className="flex flex-wrap gap-6 mb-6">
        {[
          { label: 'Workflow', id: target.workflowId },
          { label: 'Node', id: target.nodeId },
        ].map(({ label, id }) => {
          const { name, uuid } = splitIDName(id)
          return (
            <div key={label} title={id}>
              <span className="text-xs text-foreground-subtle">{label}</span>
              <div className="font-mono text-sm text-foreground">{name}</div>
              {uuid && <div className="font-mono text-xs text-foreground-muted">{uuid}</div>}
            </div>
          )
        })}
      </div>

      <div className="bg-app-surface rounded-lg shadow p-4 md:p-6 max-w-5xl">
        {target.lastError && (
          <>
            <Text size="2" weight="medium" className="block mb-1">
              Last error
            </Text>
            <pre className="bg-app-surface-muted rounded p-3 text-xs font-mono overflow-auto max-h-32 whitespace-pre-wrap break-all mb-4">
              {target.lastError}
            </pre>
          </>
        )}

        <Text size="2" weight="medium" className="block mb-1">
          Cached task result
        </Text>
        {target.cachedTaskResult ? (
          <pre className="bg-app-surface-muted rounded p-3 text-xs font-mono overflow-auto max-h-48 whitespace-pre-wrap break-all mb-4">
            {JSON.stringify(target.cachedTaskResult, null, 2)}
          </pre>
        ) : (
          <Text size="2" color="gray" className="block mb-4">
            No cached results for this node.
          </Text>
        )}

        <Text size="2" weight="medium" className="block mb-1">
          Action
        </Text>
        <div className="flex gap-3 mb-4 flex-wrap">
          {ADMIN_ACTIONS.map(({ action: candidate, label, color, disabledForGateway, description }) => {
            const disabled = target.isGateway && disabledForGateway
            return (
              <div key={candidate} className="flex items-center gap-1">
                <Button
                  type="button"
                  variant={action === candidate ? 'solid' : 'soft'}
                  color={color}
                  size="2"
                  disabled={disabled}
                  title={disabled ? 'Not supported for GATEWAY nodes — use Retry or Abort' : undefined}
                  onClick={() => setAction(candidate)}
                >
                  {label}
                </Button>
                <Tooltip content={description} maxWidth="320px">
                  <InfoCircledIcon className="text-foreground-muted cursor-help" width={15} height={15} aria-label={`What ${label} does`} />
                </Tooltip>
              </div>
            )
          })}
        </div>

        <Text size="2" weight="medium" className="block mb-1">
          Reason (required)
        </Text>
        <TextArea
          value={reason}
          onChange={(e) => setReason(e.target.value)}
          rows={2}
          placeholder="Why are you resolving this node?"
          className="mb-4"
        />

        {/* Overrides only apply to OVERRIDE and RETRY (see core's parkNodeForAdmin — SKIP always
            discards them, and ABORT never gets to signal anything) — so this only shows up once
            one of those is the chosen action. The heading reflects which direction the values
            flow: OVERRIDE merges them straight into workflow variables as the node's final
            output, skipping the handler entirely; RETRY merges them in *before* re-running the
            real handler, so they're consumed as input (e.g. what a TASK node's input_mapping
            reads, or what a GATEWAY's routing condition reads) rather than the result. */}
        {(action === 'OVERRIDE' || action === 'RETRY') && (
          <div className="mb-4">
            <div className="flex items-center justify-between mb-1">
              <Text size="2" weight="medium">
                {action === 'OVERRIDE' ? 'Override Outputs (JSON)' : 'Override Inputs (JSON)'}
              </Text>
              <Button type="button" variant="ghost" size="1" onClick={resetOverridesToCachedValue}>
                Reset to cached value
              </Button>
            </div>
            <div className="grid md:grid-cols-2 gap-3">
              <div>
                <Text size="1" color="gray" className="block mb-1">
                  Edit
                </Text>
                <LineNumberedTextArea value={overridesText} onChange={setOverridesText} />
                {overridesJSONError && (
                  <Text size="1" color="red" className="block mt-1">
                    {overridesJSONError}
                  </Text>
                )}
              </div>
              <div>
                <Text size="1" color="gray" className="block mb-1">
                  Diff vs cached value
                </Text>
                <div className="bg-app-surface-muted rounded p-3 text-xs font-mono overflow-auto h-72">
                  {overridesDiff === null ? (
                    <Text size="1" color="gray">
                      Too large to diff.
                    </Text>
                  ) : overridesDiffRows && overridesDiffRows.length === 0 ? (
                    <Text size="1" color="gray">
                      No differences from the cached value.
                    </Text>
                  ) : (
                    overridesDiffRows?.map((row, i) => {
                      if (row.kind === 'gap') {
                        return (
                          <div key={`gap-${i}`} className="text-foreground-subtle select-none py-0.5">
                            ⋯ {row.count} unchanged line{row.count === 1 ? '' : 's'}
                          </div>
                        )
                      }
                      if (row.kind === 'replace') {
                        return (
                          <Fragment key={i}>
                            <DiffLineRow oldLineNo={row.removed.oldLineNo} newLineNo={null} type="removed">
                              {row.charDiff ? renderCharDiff(row.charDiff, 'removed') : row.removed.text || ' '}
                            </DiffLineRow>
                            <DiffLineRow oldLineNo={null} newLineNo={row.added.newLineNo} type="added">
                              {row.charDiff ? renderCharDiff(row.charDiff, 'added') : row.added.text || ' '}
                            </DiffLineRow>
                          </Fragment>
                        )
                      }
                      return (
                        <DiffLineRow key={i} oldLineNo={row.line.oldLineNo} newLineNo={row.line.newLineNo} type={row.line.type}>
                          {row.line.text || ' '}
                        </DiffLineRow>
                      )
                    })
                  )}
                </div>
              </div>
            </div>
          </div>
        )}

        {error && (
          <Text size="2" color="red" className="block mb-2">
            {error}
          </Text>
        )}

        <div className="flex justify-end gap-2 mt-4">
          <Button variant="soft" color="gray" onClick={onBack} disabled={submitting}>
            Cancel
          </Button>
          <Button disabled={!action || !reason.trim() || submitting} onClick={() => setConfirmOpen(true)}>
            {submitting ? 'Submitting…' : 'Submit'}
          </Button>
        </div>

        {/* A live workflow signal, not a draft — there's no undo once it's sent (ABORT fails the
            whole workflow outright), so Submit opens this instead of calling submit() directly. */}
        <AlertDialog.Root open={confirmOpen} onOpenChange={setConfirmOpen}>
          <AlertDialog.Content maxWidth="440px">
            {action && (
              <>
                <AlertDialog.Title>Confirm {ADMIN_ACTIONS.find((a) => a.action === action)?.label}</AlertDialog.Title>
                <AlertDialog.Description size="2" className="mb-2">
                  You're about to <strong>{action}</strong> node{' '}
                  <span className="font-mono break-all">{target.nodeId}</span> on workflow{' '}
                  <span className="font-mono break-all">{target.workflowId}</span>. This takes effect immediately
                  and can't be undone.
                </AlertDialog.Description>
                <AlertDialog.Description size="2" color="gray" className="block mb-4">
                  {ADMIN_ACTIONS.find((a) => a.action === action)?.description}
                </AlertDialog.Description>
                <div className="flex justify-end gap-2">
                  <AlertDialog.Cancel>
                    <Button variant="soft" color="gray">
                      Cancel
                    </Button>
                  </AlertDialog.Cancel>
                  <AlertDialog.Action>
                    <Button
                      color={ADMIN_ACTIONS.find((a) => a.action === action)?.color}
                      onClick={() => void submit()}
                    >
                      Yes, {ADMIN_ACTIONS.find((a) => a.action === action)?.label.toLowerCase()}
                    </Button>
                  </AlertDialog.Action>
                </div>
              </>
            )}
          </AlertDialog.Content>
        </AlertDialog.Root>
      </div>
    </div>
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
// ChildWorkflowBranch). Exposes both a cache-respecting ensureFetched (used by toggle/expand-all)
// and a cache-bypassing refetch, the latter used to refresh this specific instance's rows after
// an admin resolves a node inside it.
function useExpandableWorkflow(workflowId: string, kind: WorkflowBranchKind, expandSignal: ExpandSignal) {
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
  workflowId,
  showAllNodes,
  expandSignal,
  onOpenVariables,
  onOpenResolve,
  onRefresh,
}: {
  node: EngineNode
  depth: number
  // The workflow instance this node belongs to — the root, a child branch, or a task workflow.
  // Needed to address a resolve request at the right instance, since a node id alone isn't
  // unique across the whole tree.
  workflowId: string
  showAllNodes: boolean
  expandSignal: ExpandSignal
  onOpenVariables: (target: WorkflowVariablesTarget) => void
  onOpenResolve: (target: AdminResolutionTarget) => void
  // Refreshes this node's own workflow instance (not its children) after an admin resolves it.
  onRefresh: () => void
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
            <span className="truncate" title={humanizeNodeType(node.gateway_type ?? node.type)}>
              {humanizeNodeType(node.gateway_type ?? node.type)}
            </span>
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
        <div className="px-3 flex flex-col items-start gap-1">
          <Badge color={NODE_STATUS_COLOR[node.status]}>{node.status}</Badge>
          {node.status === 'AWAITING_ADMIN' && (
            <Button
              variant="soft"
              color="amber"
              size="1"
              onClick={() =>
                onOpenResolve({
                  workflowId,
                  nodeId: node.id,
                  isGateway: node.type === 'GATEWAY',
                  lastError: node.last_error,
                  cachedTaskResult: node.cached_task_result,
                  onResolved: onRefresh,
                })
              }
            >
              Resolve
            </Button>
          )}
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
          onOpenResolve={onOpenResolve}
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
          onOpenResolve={onOpenResolve}
          onRefresh={taskBranch.refetch}
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
  onOpenResolve,
}: {
  workflowId: string
  depth: number
  showAllNodes: boolean
  expandSignal: ExpandSignal
  onOpenVariables: (target: WorkflowVariablesTarget) => void
  onOpenResolve: (target: AdminResolutionTarget) => void
}) {
  const { expanded, toggle, loading, status, error, refetch } = useExpandableWorkflow(workflowId, 'child', expandSignal)

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
          workflowId={workflowId}
          depth={depth}
          loading={loading}
          status={status}
          error={error}
          showAllNodes={showAllNodes}
          expandSignal={expandSignal}
          onOpenVariables={onOpenVariables}
          onOpenResolve={onOpenResolve}
          onRefresh={refetch}
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
  onOpenResolve,
  onRefresh,
}: {
  workflowId: string
  depth: number
  loading: boolean
  status: EngineStatus | null
  error: FetchError
  showAllNodes: boolean
  expandSignal: ExpandSignal
  onOpenVariables: (target: WorkflowVariablesTarget) => void
  onOpenResolve: (target: AdminResolutionTarget) => void
  onRefresh: () => void
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
        workflowId={workflowId}
        depth={depth}
        loading={loading}
        status={status}
        error={error}
        showAllNodes={showAllNodes}
        expandSignal={expandSignal}
        onOpenVariables={onOpenVariables}
        onOpenResolve={onOpenResolve}
        onRefresh={onRefresh}
      />
    </>
  )
}

// The loading/error/node-list body shared by ChildWorkflowBranch and TaskWorkflowPanel once
// expanded.
function WorkflowBranchBody({
  workflowId,
  depth,
  loading,
  status,
  error,
  showAllNodes,
  expandSignal,
  onOpenVariables,
  onOpenResolve,
  onRefresh,
}: {
  workflowId: string
  depth: number
  loading: boolean
  status: EngineStatus | null
  error: FetchError
  showAllNodes: boolean
  expandSignal: ExpandSignal
  onOpenVariables: (target: WorkflowVariablesTarget) => void
  onOpenResolve: (target: AdminResolutionTarget) => void
  onRefresh: () => void
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
              workflowId={workflowId}
              showAllNodes={showAllNodes}
              expandSignal={expandSignal}
              onOpenVariables={onOpenVariables}
              onOpenResolve={onOpenResolve}
              onRefresh={onRefresh}
            />
          ))
        ))}
    </div>
  )
}
