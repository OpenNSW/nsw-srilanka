import { useCallback, useEffect, useState } from 'react'
import { useParams } from 'react-router-dom'
import { Badge, Button, Spinner, Text } from '@radix-ui/themes'
import { ReloadIcon } from '@radix-ui/react-icons'
import { getConsignmentEngineStatus } from '@/features/admin/service'
import type { EngineNodeStatus, EngineStatus, EngineWorkflowStatus } from '@/features/admin/types'

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

function formatDateTime(iso: string): string {
  const d = new Date(iso)
  return Number.isNaN(d.getTime()) ? iso : d.toLocaleString()
}

// Internal ops view of a consignment's raw engine state — the same picture you'd
// otherwise need the Temporal UI for. Not gated by an admin role yet (backend
// TODO: OpenNSW/nsw-srilanka HandleGetConsignmentEngineStatus); reachable only by
// direct URL, not linked from trader/CHA navigation.
export function AdminConsignmentEngineStatusScreen() {
  const { consignmentId } = useParams<{ consignmentId: string }>()
  const [status, setStatus] = useState<EngineStatus | null>(null)
  const [loading, setLoading] = useState(true)
  const [refreshing, setRefreshing] = useState(false)
  const [error, setError] = useState<'notFound' | 'loadFailed' | null>(null)

  const refresh = useCallback(async () => {
    if (!consignmentId) return
    setRefreshing(true)
    try {
      const result = await getConsignmentEngineStatus(consignmentId)
      setStatus(result)
      setError(result ? null : 'notFound')
    } catch (err) {
      console.error('Failed to fetch consignment engine status:', err)
      setError('loadFailed')
    } finally {
      setRefreshing(false)
    }
  }, [consignmentId])

  // Initial load is inlined (rather than reusing `refresh`) so the effect body
  // itself never calls a setState setter synchronously — only from within the
  // promise continuation.
  useEffect(() => {
    if (!consignmentId) return
    let cancelled = false
    getConsignmentEngineStatus(consignmentId)
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
  }, [consignmentId])

  if (!consignmentId) {
    return (
      <div className="p-6">
        <Text color="red">A consignment ID is required.</Text>
      </div>
    )
  }

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
      <div className="mb-4 flex items-center justify-between">
        <div className="flex items-center gap-3">
          <h1 className="text-xl font-semibold text-foreground">Engine status</h1>
          <Badge size="2" color={WORKFLOW_STATUS_COLOR[status.status]}>
            {status.status}
          </Badge>
        </div>
        <Button variant="soft" color="blue" size="2" onClick={() => void refresh()} disabled={refreshing}>
          <ReloadIcon className={refreshing ? 'animate-spin' : ''} />
          Refresh
        </Button>
      </div>

      <p className="text-xs font-mono text-foreground-muted mb-6">{status.consignment_id}</p>

      <div className="bg-app-surface rounded-lg shadow mb-6">
        <table className="w-full text-sm">
          <thead>
            <tr className="text-left border-b border-app-border">
              <th className="p-3 font-medium text-foreground-subtle">Node</th>
              <th className="p-3 font-medium text-foreground-subtle">Type</th>
              <th className="p-3 font-medium text-foreground-subtle">Status</th>
              <th className="p-3 font-medium text-foreground-subtle">Updated</th>
              <th className="p-3 font-medium text-foreground-subtle">Last error</th>
            </tr>
          </thead>
          <tbody>
            {status.nodes.length === 0 ? (
              <tr>
                <td className="p-3 text-foreground-muted" colSpan={5}>
                  No nodes reported yet.
                </td>
              </tr>
            ) : (
              status.nodes.map((node) => (
                <tr key={node.id} className="border-b border-app-border last:border-0">
                  <td className="p-3 font-mono text-xs">{node.id}</td>
                  <td className="p-3">{node.type}</td>
                  <td className="p-3">
                    <Badge color={NODE_STATUS_COLOR[node.status]}>{node.status}</Badge>
                  </td>
                  <td className="p-3 text-foreground-muted">{formatDateTime(node.updated_at)}</td>
                  <td className="p-3 text-red-600">{node.last_error ?? ''}</td>
                </tr>
              ))
            )}
          </tbody>
        </table>
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
    </div>
  )
}
