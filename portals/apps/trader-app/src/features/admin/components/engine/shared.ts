import type { AdminWorkflowKind, EngineEdge, EngineNode, ParkCategory } from '@/features/admin/types'

// Shared grid so NodeRow's columns line up under the header regardless of nesting depth.
export const NODE_ROW_GRID = 'grid grid-cols-[1fr_150px_130px_150px_1fr] gap-2 items-center'

// START/END carry no ops-actionable signal of their own — whether a workflow reached END is
// already visible from its own status badge (COMPLETED), and a START simply means "this
// execution began," true of every non-empty node list. Hidden by default at every nesting level
// (root, child branches, task workflows alike) to cut noise; the eye toggle in the header shows
// them all when actually needed (e.g. checking a START/END's own timestamp).
const NOISY_NODE_TYPES = new Set(['START', 'END'])

export function visibleNodes(nodes: EngineNode[], showAllNodes: boolean): EngineNode[] {
  return showAllNodes ? nodes : nodes.filter((node) => !NOISY_NODE_TYPES.has(node.type))
}

// A one-shot "expand everything"/"collapse everything" command from the toolbar, threaded down
// to every useExpandableWorkflow instance. gen === 0 means no command has been issued yet — a
// plain boolean can't represent that third state, and without it every branch would eagerly
// expand and fetch on mount before the toggle was ever clicked. Branches lazily fetch on first
// expand, so a branch revealed only after its parent's fetch completes must also pick up the
// current signal on mount, not just the ones that existed at click time — see
// useExpandableWorkflow's effect.
export interface ExpandSignal {
  gen: number
  expand: boolean
}

export const NO_EXPAND_SIGNAL: ExpandSignal = { gen: 0, expand: false }

// A fetch either found the workflow, didn't (404 — normal for a not-yet-started or already-gone
// execution), or failed for some other reason.
export type FetchError = 'notFound' | 'loadFailed' | null

// Identifies which workflow instance's global variables are open in the dialog — the flow on
// screen, or one of its expanded child branches. Each workflow instance has its
// own independent WorkflowVariables snapshot, so the dialog is always scoped to exactly one.
export interface WorkflowVariablesTarget {
  workflowId: string
  label: string
  variables?: Record<string, unknown>
}

// Identifies the node an admin is resolving, plus which workflow instance it belongs to (root,
// a child branch, or a task workflow — see NodeRow) and how to refresh that instance's view once
// resolved. lastError, parkCategory, the mappings and cachedTaskResult are carried along purely so
// the resolve view can show them without a second fetch — see ResolveAdminInterventionView.
export interface AdminResolutionTarget {
  workflowId: string
  // Which route family addresses workflowId (see AdminWorkflowKind): the resolve request goes to a
  // different endpoint for a task workflow than for the root or a child branch.
  workflowKind: AdminWorkflowKind
  nodeId: string
  // The parking being resolved (EngineNode.step_id).
  stepId: string
  isGateway: boolean
  lastError?: string
  parkCategory?: ParkCategory
  inputMapping?: Record<string, string>
  outputMapping?: Record<string, string>
  cachedTaskResult?: Record<string, unknown>
  // The variables of the workflow instance the node is in (the one at workflowId), so an admin can
  // see what a Retry will read and what a patch will change without going back to the debugger.
  globalVariables?: Record<string, unknown>
  // This node's own outgoing edges, condition included — only meaningful (and only shown) for a
  // GATEWAY, where "no matching conditions" is otherwise a dead end: the admin has no way to see
  // what each edge actually checks without this.
  outgoingEdges?: EngineEdge[]
  onResolved: () => void
}
