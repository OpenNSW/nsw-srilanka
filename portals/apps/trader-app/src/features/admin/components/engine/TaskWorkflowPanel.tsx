import type { EngineStatus } from '@/features/admin/types'
import { WorkflowBranchBody } from '@/features/admin/components/engine/WorkflowBranchBody'
import type {
  ExpandSignal,
  FetchError,
  WorkflowVariablesTarget,
  AdminResolutionTarget,
} from '@/features/admin/components/engine/shared'
import { GlobalVariablesButton } from '@/features/admin/components/variables/GlobalVariablesButton'

// The expanded contents of a task workflow toggled open from NodeRow's inline affordance — a
// lighter-weight, un-boxed counterpart to ChildWorkflowBranch's tinted card, since a task
// workflow is expected on nearly every TASK node rather than being an occasional structural
// branch. Still gets its own "Global variables" action and ID, just without the full-line
// always-visible toggle bar.
export function TaskWorkflowPanel({
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
        className="border-l-2 border-border flex items-center justify-between pr-3 py-1 pl-2"
        style={{ marginLeft: 12 + depth * 20 }}
      >
        <span className="text-[11px] font-mono text-foreground-muted truncate" title={workflowId}>
          <span className="text-foreground-muted">task workflow </span>
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
        workflowKind="task"
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
