import { Link, useParams, useSearchParams } from 'react-router-dom'
import { Button, Text } from '@radix-ui/themes'
import { PageState } from '@portals/ui'
import { useAdminConsignment } from '@/features/admin/hooks'
import { formatDateTime } from '@/features/consignment/utils'
import { isAgencyMode } from '@/features/case/mode'
import { EngineStatusView } from '@/features/admin/components/engine/EngineStatusView'
import { FlowList } from '@/features/admin/components/flows/FlowList'
import { ROUTES } from '@/constants'
import { ConsignmentStateBadge } from '@/features/consignment/components/ConsignmentStateBadge'
import { TradeFlowBadge } from '@/features/consignment/components/TradeFlowBadge'

// Internal ops view of a consignment or case: its flows, each with its raw engine state — the
// same picture you'd otherwise need the Temporal UI for. Backend gates this behind the
// ConsignmentAdminRead scope (see internal/workflowadmin.CaseService and
// HandleGetWorkflowEngineStatus); reachable only by direct URL, not linked from
// trader/CHA/officer navigation.
//
// The flow on screen is ?flow=<workflowId> (so a link can point at one), else the first flow,
// which in TNSW is the export flow.
export function AdminConsignmentEngineStatusScreen() {
  const { consignmentId } = useParams<{ consignmentId: string }>()
  const { consignment, error, loading, refetch } = useAdminConsignment(consignmentId)
  const [searchParams, setSearchParams] = useSearchParams()

  if (!consignmentId) {
    return (
      <div className="p-6">
        <Text color="red">{isAgencyMode ? 'A case ID is required.' : 'A consignment ID is required.'}</Text>
      </div>
    )
  }

  if (loading) {
    return (
      <div className="p-6">
        <PageState variant="loading" title={isAgencyMode ? 'Loading case…' : 'Loading consignment…'} />
      </div>
    )
  }

  if (error || !consignment) {
    return (
      <div className="p-6">
        <div className="bg-app-surface rounded-lg shadow">
          <PageState
            variant="error"
            title={
              error === 'notFound'
                ? isAgencyMode
                  ? 'Case not found'
                  : 'Consignment not found'
                : isAgencyMode
                  ? 'Failed to load case'
                  : 'Failed to load consignment'
            }
            onRetry={refetch}
          />
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
      <div className="mb-1 flex flex-wrap items-center justify-between gap-2">
        <h1 className="text-xl font-semibold text-foreground">
          {isAgencyMode ? 'Case Debugger' : 'Consignment Debugger'}
        </h1>
        {!isAgencyMode && (
          <Button variant="soft" color="gray" size="2" asChild>
            <Link to={ROUTES.ADMIN_CONSIGNMENT_VIEW(consignmentId)}>View consignment</Link>
          </Button>
        )}
      </div>

      <div className="mb-2 flex items-center gap-2 flex-wrap">
        <Text size="3" weight="medium" className="text-foreground">
          {consignment.name || (isAgencyMode ? 'Untitled case' : 'Untitled consignment')}
        </Text>
        <ConsignmentStateBadge state={consignment.state} />
        {!isAgencyMode && <TradeFlowBadge flow={consignment.flow} />}
      </div>

      <div className="mb-6 flex items-center gap-4 flex-wrap">
        <p className="text-xs font-mono text-foreground-muted">{consignmentId}</p>
        <p className="text-xs text-foreground-muted">Created {formatDateTime(consignment.createdAt)}</p>
        {!isAgencyMode && (
          <p className="text-xs text-foreground-muted">
            Trader <span className="font-mono">{consignment.traderId}</span>
          </p>
        )}
      </div>

      <FlowList
        flows={consignment.flows}
        selectedId={selectedId}
        onSelect={(workflowId) => setSearchParams({ flow: workflowId })}
      />

      {selectedId ? (
        // Keyed on the flow so switching flows also resets the view's own state: an open resolve
        // screen or variables dialog, the expand-all toggle and the START/END toggle.
        <EngineStatusView
          key={selectedId}
          workflowId={selectedId}
          title={selectedFlow?.templateName ?? selectedId}
          onRefresh={refetch}
        />
      ) : (
        <Text size="2" color="gray">
          No flow has started on this {isAgencyMode ? 'case' : 'consignment'}.
        </Text>
      )}
    </div>
  )
}
