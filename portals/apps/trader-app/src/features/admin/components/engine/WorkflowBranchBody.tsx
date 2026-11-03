import { Spinner, Text } from '@radix-ui/themes'
import type { AdminWorkflowKind, EngineStatus } from '@/features/admin/types'
import { NodeRow } from '@/features/admin/components/engine/NodeRow'
import {
  visibleNodes,
  type ExpandSignal,
  type FetchError,
  type WorkflowVariablesTarget,
  type AdminResolutionTarget,
} from '@/features/admin/components/engine/shared'

// The loading/error/node-list body shared by ChildWorkflowBranch and TaskWorkflowPanel once
// expanded.
export function WorkflowBranchBody({
  workflowId,
  workflowKind,
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
  // The kind of workflow this body lists the nodes of; each NodeRow passes it on when opening resolve.
  workflowKind: AdminWorkflowKind
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
              workflowKind={workflowKind}
              edges={status.edges ?? []}
              variables={status.global_variables}
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
