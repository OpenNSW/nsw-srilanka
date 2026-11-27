import { isAgencyMode } from '@/features/case/mode'
import { ConsignmentSearchCard, type SearchCopy } from '@/features/admin/components/search/ConsignmentSearchCard'

const CONSIGNMENT_COPY: SearchCopy = {
  title: 'Consignment search',
  subtitle: 'Open a consignment a trader has referred you to.',
  idLabel: 'Consignment ID',
  helper: 'Traders find this ID on their consignment list and detail screens. Exact match only.',
  notFound: 'Consignment not found. Check the ID with the trader.',
  error: "Couldn't look up the consignment. Please try again.",
}

const CASE_COPY: SearchCopy = {
  title: 'Case search',
  subtitle: 'Open a case you have been referred to.',
  idLabel: 'Case ID',
  helper: 'The ID is shown on the case list and detail screens. Exact match only.',
  notFound: 'Case not found. Check the ID and try again.',
  error: "Couldn't look up the case. Please try again.",
}

const SEARCH_COPY = isAgencyMode ? CASE_COPY : CONSIGNMENT_COPY

// The admin's landing page (/admin), where an admin opens a consignment, or an agency's case,
// that someone referred them to. Admins can't browse or list them: every lookup starts from an
// ID. Submitting doesn't do anything yet: opening the debugger comes next (#1326).
export function AdminDashboardScreen() {
  return (
    <div className="p-4 md:p-6">
      <div className="max-w-2xl">
        <h1 className="text-xl font-semibold text-foreground">{SEARCH_COPY.title}</h1>
        <p className="mt-1 mb-4 text-sm text-foreground-muted">{SEARCH_COPY.subtitle}</p>
        <ConsignmentSearchCard copy={SEARCH_COPY} onSearch={() => {}} status="idle" />
      </div>
    </div>
  )
}
