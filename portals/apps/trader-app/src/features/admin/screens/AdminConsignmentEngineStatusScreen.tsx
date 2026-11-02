import { useParams } from 'react-router-dom'
import { Text } from '@radix-ui/themes'
import { ConsignmentDebugger } from '@/features/admin/components/flows/ConsignmentDebugger'
import { ACCESS_SCOPES } from '@/constants/scopes'
import { useScopes } from '@/hooks/useScopes'

// Internal ops view of a consignment: its flows, each with its raw engine state — the same
// picture you'd otherwise need the Temporal UI for. Backend gates this behind the
// ConsignmentAdminRead scope (see HandleAdminGetConsignmentByID and
// HandleGetWorkflowEngineStatus); reachable only by direct URL, not linked from trader/CHA
// navigation.
export function AdminConsignmentEngineStatusScreen() {
  const { consignmentId } = useParams<{ consignmentId: string }>()
  // Resolving and force-parking take the admin actions scope; without it the view only reports.
  const canIntervene = useScopes().hasScope(ACCESS_SCOPES.ADMIN_ACTIONS)

  if (!consignmentId) {
    return (
      <div className="p-6">
        <Text color="red">A consignment ID is required.</Text>
      </div>
    )
  }

  // Keyed on consignmentId so navigating to a different consignment remounts fresh.
  return <ConsignmentDebugger key={consignmentId} consignmentId={consignmentId} canIntervene={canIntervene} />
}
