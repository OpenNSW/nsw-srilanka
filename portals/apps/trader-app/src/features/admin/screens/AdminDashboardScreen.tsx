import { isAgencyMode } from '@/features/case/mode'

// The admin's landing page (/admin), where an admin opens a consignment, or an agency's case,
// that someone referred them to. Admins can't browse or list them: every lookup starts from an
// ID. The search itself is still to come (#1326).
export function AdminDashboardScreen() {
  return (
    <div className="p-4 md:p-6">
      <div className="max-w-2xl">
        <h1 className="text-xl font-semibold text-foreground">{isAgencyMode ? 'Case search' : 'Consignment search'}</h1>
      </div>
    </div>
  )
}
