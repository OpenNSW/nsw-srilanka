import { useParams } from 'react-router-dom'
import { Text } from '@radix-ui/themes'
import { ConsignmentDebugger } from '@/features/admin/components/flows/ConsignmentDebugger'

// Internal ops view of a consignment: its flows, each with its raw engine state — the same
// picture you'd otherwise need the Temporal UI for. Backend gates this behind the
// ConsignmentAdminRead scope (see HandleAdminGetConsignmentByID and
// HandleGetWorkflowEngineStatus); reachable only by direct URL, not linked from trader/CHA
// navigation.
export function AdminConsignmentEngineStatusScreen() {
  const { consignmentId } = useParams<{ consignmentId: string }>()

  if (!consignmentId) {
    return (
      <div className="p-6">
        <Text color="red">A consignment ID is required.</Text>
      </div>
    )
  }

  // Keyed on consignmentId so navigating to a different consignment remounts fresh.
  return <ConsignmentDebugger key={consignmentId} consignmentId={consignmentId} />
}
