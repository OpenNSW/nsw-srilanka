import { useCallback, useEffect, useState } from 'react'
import { Link, useSearchParams } from 'react-router-dom'
import { Badge, Button, Spinner, Text } from '@radix-ui/themes'
import { getConsignmentForAdmin } from '@/features/admin/service'
import type { AdminConsignmentDetail } from '@/features/admin/types'
import { formatDateTime, formatState, getStateColor } from '@/features/consignment/utils'
import { EngineStatusView } from '@/features/admin/components/engine/EngineStatusView'
import type { FetchError } from '@/features/admin/components/engine/shared'
import { FlowList } from '@/features/admin/components/flows/FlowList'

// The consignment's business-side detail and its flows. The flow on screen is ?flow=<workflowId>
// (so a link can point at one), else the first flow, which is the export flow.
export function ConsignmentDebugger({ consignmentId }: { consignmentId: string }) {
  const [consignment, setConsignment] = useState<AdminConsignmentDetail | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<FetchError>(null)
  const [searchParams, setSearchParams] = useSearchParams()

  const reload = useCallback(async () => {
    try {
      const result = await getConsignmentForAdmin(consignmentId)
      setConsignment(result)
      setError(result ? null : 'notFound')
    } catch (err) {
      console.error('Failed to fetch consignment:', err)
      setError('loadFailed')
    }
  }, [consignmentId])

  useEffect(() => {
    let cancelled = false
    getConsignmentForAdmin(consignmentId)
      .then((result) => {
        if (cancelled) return
        setConsignment(result)
        setError(result ? null : 'notFound')
      })
      .catch((err: unknown) => {
        if (cancelled) return
        console.error('Failed to fetch consignment:', err)
        setError('loadFailed')
      })
      .finally(() => {
        if (!cancelled) setLoading(false)
      })
    return () => {
      cancelled = true
    }
  }, [consignmentId])

  if (loading) {
    return (
      <div className="p-6 flex items-center justify-center py-12">
        <Spinner size="3" />
        <Text size="3" color="gray" className="ml-3">
          Loading consignment…
        </Text>
      </div>
    )
  }

  if (error || !consignment) {
    return (
      <div className="p-6">
        <div className="bg-app-surface rounded-lg shadow p-8 text-center">
          <Text size="5" color="red" weight="medium" className="block mb-2">
            {error === 'notFound' ? 'Consignment not found' : 'Failed to load consignment'}
          </Text>
          <Button variant="soft" onClick={() => void reload()}>
            Try again
          </Button>
        </div>
      </div>
    )
  }

  // ?flow= selects only among this consignment's flows; any other ID falls back to the
  // first flow, so another workflow's nodes never show under this consignment.
  const requestedId = searchParams.get('flow')
  const selectedFlow = consignment.flows.find((flow) => flow.workflowId === requestedId) ?? consignment.flows[0]
  const selectedId = selectedFlow?.workflowId ?? null

  return (
    <div className="p-4 md:p-6">
      <div className="mb-1 flex items-center justify-between">
        <h1 className="text-xl font-semibold text-foreground">Consignment Debugger</h1>
        <Button variant="soft" color="gray" size="2" asChild>
          <Link to={`/admin/consignments/${consignmentId}/view`}>View consignment</Link>
        </Button>
      </div>

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

      <div className="mb-6 flex items-center gap-4 flex-wrap">
        <p className="text-xs font-mono text-foreground-muted">{consignmentId}</p>
        <p className="text-xs text-foreground-muted">Created {formatDateTime(consignment.createdAt)}</p>
        <p className="text-xs text-foreground-muted">
          Trader <span className="font-mono">{consignment.traderId}</span>
        </p>
      </div>

      <FlowList
        flows={consignment.flows}
        selectedId={selectedId}
        onSelect={(workflowId) => setSearchParams({ flow: workflowId })}
      />

      {selectedId ? (
        <EngineStatusView
          key={selectedId}
          workflowId={selectedId}
          title={selectedFlow?.templateName ?? selectedId}
          onRefresh={() => void reload()}
        />
      ) : (
        <Text size="2" color="gray">
          No flow has started on this consignment.
        </Text>
      )}
    </div>
  )
}
