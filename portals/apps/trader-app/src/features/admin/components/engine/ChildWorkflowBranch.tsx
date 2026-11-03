import { ChevronRightIcon } from '@radix-ui/react-icons'
import { WorkflowBranchBody } from '@/features/admin/components/engine/WorkflowBranchBody'
import type {
  ExpandSignal,
  WorkflowVariablesTarget,
  AdminResolutionTarget,
} from '@/features/admin/components/engine/shared'
import { useExpandableWorkflow } from '@/features/admin/components/engine/useExpandableWorkflow'
import { GlobalVariablesButton } from '@/features/admin/components/variables/GlobalVariablesButton'

// A collapsed-by-default full-width row for one native engine child workflow (SPLIT_TASK/
// BATCH_SPLIT/PARALLEL_SPLIT). Fetches its engine status only on first expand, and keeps the
// result cached in local state so collapsing and re-expanding doesn't re-fetch. Once fetched,
// exposes a "Global variables" action scoped to this specific workflow instance.
export function ChildWorkflowBranch({
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
            <span className="text-foreground-muted">child workflow</span>
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
          workflowKind="workflow"
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
