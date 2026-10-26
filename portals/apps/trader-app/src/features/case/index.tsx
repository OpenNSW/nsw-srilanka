// Agency mode: the same app, deployed for an agency, lists and opens cases instead of
// consignments. The screens mount at /cases (see ROUTES), with the routes' params still
// named after consignments.
import { ConsignmentDetailScreen } from '@/features/consignment/screens/ConsignmentDetailScreen.tsx'
import { getCase } from './service'

export { CaseListScreen } from './CaseListScreen'

export function CaseDetailScreen() {
  return <ConsignmentDetailScreen fetcher={getCase} />
}
