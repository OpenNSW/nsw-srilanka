import { useEffect, useRef, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { isAgencyMode } from '@/features/case/mode'
import { ROUTES } from '@/constants'
import { useAdminConsignmentLookup } from '@/features/admin/hooks'
import {
  ConsignmentSearchCard,
  type SearchCopy,
  type SearchStatus,
} from '@/features/admin/components/search/ConsignmentSearchCard'

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
// ID. A search looks the ID up first, so a missing ID or a failed lookup is reported here; a
// found one opens the debugger, at /admin/consignments/{id} in TNSW or /admin/cases/{id} in an
// agency.
export function AdminDashboardScreen() {
  const navigate = useNavigate()
  const { lookup, isPending } = useAdminConsignmentLookup()
  const [status, setStatus] = useState<SearchStatus>('idle')
  const lastIdRef = useRef('')
  // Set while a lookup runs, so a second submit before the re-render can't start another.
  const inFlightRef = useRef(false)
  // False once the screen unmounts, so a lookup that settles later leaves it alone.
  const mountedRef = useRef(true)
  useEffect(() => {
    mountedRef.current = true
    return () => {
      mountedRef.current = false
    }
  }, [])

  async function search(id: string) {
    if (inFlightRef.current) return
    inFlightRef.current = true
    lastIdRef.current = id
    setStatus('idle')
    try {
      const found = await lookup(id)
      if (!mountedRef.current) return
      // Encoded so an ID like "a/b" can't open another route.
      if (found) void navigate(ROUTES.ADMIN_CONSIGNMENT(encodeURIComponent(id)))
      else setStatus('notFound')
    } catch {
      if (mountedRef.current) setStatus('error')
    } finally {
      inFlightRef.current = false
    }
  }

  return (
    <div className="p-4 md:p-6">
      <div className="max-w-2xl">
        <h1 className="text-xl font-semibold text-foreground">{SEARCH_COPY.title}</h1>
        <p className="mt-1 mb-4 text-sm text-foreground-muted">{SEARCH_COPY.subtitle}</p>
        <ConsignmentSearchCard
          copy={SEARCH_COPY}
          onSearch={(id) => void search(id.trim())}
          pending={isPending}
          status={status}
          onRetry={() => void search(lastIdRef.current)}
          onChange={() => setStatus('idle')}
        />
      </div>
    </div>
  )
}
