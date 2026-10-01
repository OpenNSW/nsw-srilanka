import { useParams } from 'react-router-dom'
import { ConsignmentDetailScreen } from '@/features/consignment/screens/ConsignmentDetailScreen'
import { getConsignmentForAdmin } from '@/features/admin/service'

// Renders the same consignment detail view as the trader/CHA-facing screen, but sourced via
// getConsignmentForAdmin (ConsignmentAdminRead, no ownership check) instead of getConsignment —
// so it works for an admin inspecting a consignment outside their own company, which the
// ownership-checked endpoint would 404/403 on. Linked from the "View consignment" action on
// AdminConsignmentEngineStatusScreen.
export function AdminConsignmentDetailScreen() {
  const { consignmentId } = useParams<{ consignmentId: string }>()
  return (
    <ConsignmentDetailScreen
      fetcher={getConsignmentForAdmin}
      backTo={`/admin/consignments/${consignmentId}`}
      backLabel="Back to engine status"
    />
  )
}
