import { useState } from 'react'
import { Badge, Button } from '@radix-ui/themes'
import { ChevronRightIcon } from '@radix-ui/react-icons'
import type { AdminWorkflowKind, EngineEdge, EngineNode, EngineNodeStatus } from '@/features/admin/types'
import { formatDateTime } from '@/features/consignment/utils'
import { humanizeStatus } from '@/utils/formatStatus'
import { ChildWorkflowBranch } from '@/features/admin/components/engine/ChildWorkflowBranch'
import { ErrorCell } from '@/features/admin/components/engine/ErrorCell'
import { ForceParkDialog } from '@/features/admin/components/engine/ForceParkDialog'
import { TaskWorkflowPanel } from '@/features/admin/components/engine/TaskWorkflowPanel'
import {
  NODE_ROW_GRID,
  type ExpandSignal,
  type WorkflowVariablesTarget,
  type AdminResolutionTarget,
} from '@/features/admin/components/engine/shared'
import { useExpandableWorkflow } from '@/features/admin/components/engine/useExpandableWorkflow'

const NODE_STATUS_COLOR: Record<EngineNodeStatus, 'gray' | 'orange' | 'green' | 'red' | 'amber'> = {
  NOT_STARTED: 'gray',
  RUNNING: 'orange',
  COMPLETED: 'green',
  FAILED: 'red',
  AWAITING_ADMIN: 'amber',
}

// One engine node's row. A native engine child (SPLIT_TASK/BATCH_SPLIT/PARALLEL_SPLIT) gets a
// full-width expandable branch below, since those are rare and structurally significant. A TASK
// node's own task workflow (task_workflow_id) is common — nearly every TASK node that has
// started has one — so it gets a small inline toggle in the row instead of a line of its own,
// only expanding into the fuller view below on demand.
export function NodeRow({
  node,
  depth,
  workflowId,
  workflowKind,
  edges,
  variables,
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
  // Which route family addresses workflowId; passed on to the resolve view.
  workflowKind: AdminWorkflowKind
  // This workflow instance's full edge list (same instance as workflowId, not the whole tree) —
  // filtered down to node's own outgoing edges when opening the resolve view, so a GATEWAY parked
  // on "no matching conditions" can show an admin exactly what each edge checks.
  edges: EngineEdge[]
  // This workflow instance's global variables (same instance as workflowId), passed on to the
  // resolve view.
  variables?: Record<string, unknown>
  showAllNodes: boolean
  expandSignal: ExpandSignal
  onOpenVariables: (target: WorkflowVariablesTarget) => void
  onOpenResolve: (target: AdminResolutionTarget) => void
  // Refreshes this node's own workflow instance (not its children) after an admin resolves it.
  onRefresh: () => void
}) {
  const childIds = node.child_workflow_ids ?? []
  const taskBranch = useExpandableWorkflow(node.task_workflow_id ?? '', 'task', expandSignal)
  const [forceParkOpen, setForceParkOpen] = useState(false)
  // Only a TASK node that is waiting can be force-parked: on a consignment workflow that closes its
  // task, in a task workflow it parks just that step (see forceParkNode).
  const canForcePark = node.type === 'TASK' && node.status === 'RUNNING' && !!node.step_id

  // Indentation lives only on the Node cell's own padding, never on the grid row/container
  // itself — the grid's own column tracks (Type/Status/Updated/Last error) are fixed-width, so
  // padding on the row would push every one of them right by the same amount, throwing nested
  // rows out of sync with the header and with sibling rows at a different depth.
  return (
    <div className="border-b border-border last:border-0">
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
            <span className="truncate" title={humanizeStatus(node.gateway_type ?? node.type)}>
              {humanizeStatus(node.gateway_type ?? node.type)}
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
                  workflowKind,
                  nodeId: node.id,
                  stepId: node.step_id ?? '',
                  isGateway: node.type === 'GATEWAY',
                  lastError: node.last_error,
                  parkCategory: node.park_category,
                  inputMapping: node.input_mapping,
                  outputMapping: node.output_mapping,
                  cachedTaskResult: node.cached_task_result,
                  globalVariables: variables,
                  outgoingEdges: edges.filter((e) => e.source_id === node.id),
                  onResolved: onRefresh,
                })
              }
            >
              Resolve
            </Button>
          )}
          {canForcePark && (
            <>
              <Button variant="soft" color="gray" size="1" onClick={() => setForceParkOpen(true)}>
                Force park
              </Button>
              <ForceParkDialog
                open={forceParkOpen}
                onOpenChange={setForceParkOpen}
                workflowId={workflowId}
                workflowKind={workflowKind}
                nodeId={node.id}
                stepId={node.step_id ?? ''}
                onParked={onRefresh}
              />
            </>
          )}
        </div>
        <div className="px-3 text-xs text-foreground-muted">{formatDateTime(node.updated_at)}</div>
        <div className="px-3 text-xs text-error-strong min-w-0">
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
