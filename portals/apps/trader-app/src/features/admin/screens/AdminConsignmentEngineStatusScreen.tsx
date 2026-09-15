import { useCallback, useEffect, useState } from 'react'
import { Link, useParams } from 'react-router-dom'
import { Badge, Button, Dialog, Spinner, Text } from '@radix-ui/themes'
import { ReloadIcon } from '@radix-ui/react-icons'
import { getConsignmentEngineStatus, getConsignmentForAdmin } from '@/features/admin/service'
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
  const [error, setError] = useState<'notFound' | 'loadFailed' | null>(null)
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
          <Button
            variant="soft"
            color="gray"
            size="2"
            onClick={() =>
              setVariablesTarget({ workflowId, label: 'Root workflow', variables: status.global_variables })
            }
          >
            Global variables
          </Button>
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
            status.nodes.map((node) => <NodeRow key={node.id} node={node} depth={0} onOpenVariables={setVariablesTarget} />)
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
function WorkflowVariablesDialog({
  target,
  onClose,
}: {
  target: WorkflowVariablesTarget | null
  onClose: () => void
}) {
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

// One engine node's row, plus (indented below) an expandable branch per spawned child
// workflow — SPLIT_TASK/BATCH_SPLIT nodes can have several.
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
  return (
    <div className="border-b border-app-border last:border-0">
      <div className={`${NODE_ROW_GRID} py-2`} style={{ paddingLeft: depth * 20 }}>
        <div className="px-3 font-mono text-xs truncate" title={node.id}>
          {node.id}
        </div>
        <div className="px-3 text-sm">{node.gateway_type ?? node.type}</div>
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
    </div>
  )
}

// A collapsed-by-default row for one child workflow. Fetches its engine status only on first
// expand (not eagerly with the parent), and keeps the result cached in local state so
// collapsing and re-expanding doesn't re-fetch. Once fetched, exposes a "Global variables"
// action scoped to this specific child workflow instance (distinct from the root's).
function ChildWorkflowBranch({
  workflowId,
  depth,
  onOpenVariables,
}: {
  workflowId: string
  depth: number
  onOpenVariables: (target: WorkflowVariablesTarget) => void
}) {
  const [expanded, setExpanded] = useState(false)
  const [fetched, setFetched] = useState(false)
  const [loading, setLoading] = useState(false)
  const [status, setStatus] = useState<EngineStatus | null>(null)
  const [error, setError] = useState<'notFound' | 'loadFailed' | null>(null)

  const ensureFetched = useCallback(() => {
    if (fetched || loading) return
    setLoading(true)
    getConsignmentEngineStatus(workflowId)
      .then((result) => {
        setStatus(result)
        setError(result ? null : 'notFound')
        setFetched(true)
      })
      .catch((err: unknown) => {
        console.error('Failed to fetch child workflow status:', err)
        setError('loadFailed')
      })
      .finally(() => {
        setLoading(false)
      })
  }, [workflowId, fetched, loading])

  const toggle = () => {
    setExpanded((prev) => {
      const next = !prev
      if (next) ensureFetched()
      return next
    })
  }

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
            <Button
              variant="ghost"
              color="gray"
              size="1"
              onClick={() =>
                onOpenVariables({ workflowId, label: 'Child workflow', variables: status.global_variables })
              }
            >
              Global variables
            </Button>
          )}
        </div>

        {expanded && (
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
        )}
      </div>
    </div>
  )
}
