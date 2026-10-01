// Agency mode: the same app, deployed for an agency, lists and opens cases instead of
// consignments. The screens mount at the /consignments paths so the navigation that
// existing components hardcode (task cards, the task screen's back button) keeps working.
import { getEnv } from '@/runtimeConfig'
import { ConsignmentDetailScreen } from '@/features/consignment/screens/ConsignmentDetailScreen.tsx'
import { getCase } from './service'

export { CaseListScreen } from './CaseListScreen'

export const isAgencyMode = getEnv('APP_MODE', 'tnsw') === 'agency'

export function CaseDetailScreen() {
  return <ConsignmentDetailScreen fetcher={getCase} />
}
