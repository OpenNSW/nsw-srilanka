import { useEffect, useRef, useState } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import { Badge, Button, Spinner, Text } from '@radix-ui/themes'
import { useTranslation } from 'react-i18next'
import { getStateColor, formatState, formatDateTime } from '@/features/consignment/utils.ts'
import { PaginationControl } from '@/components/common/PaginationControl.tsx'
import { CONTENT_TOP_PX } from '@/components/Layout'
import type { CaseSummary } from './types'
import { getCases } from './service'

// The officer's list of cases. A trimmed ConsignmentScreen: no create button, no
// trade-flow filter, no role switch — an officer sees every case.
export function CaseListScreen() {
  const navigate = useNavigate()
  const { t } = useTranslation()
  const [cases, setCases] = useState<CaseSummary[]>([])
  const [totalCount, setTotalCount] = useState(0)
  const [loading, setLoading] = useState(true)
  const [failed, setFailed] = useState(false)
  const [page, setPage] = useState(0)
  const [retryCount, setRetryCount] = useState(0)
  const limit = 50
  const requestIdRef = useRef(0)

  useEffect(() => {
    async function fetchCases() {
      const requestId = ++requestIdRef.current
      setLoading(true)
      try {
        const data = await getCases(page * limit, limit)
        if (requestId !== requestIdRef.current) return
        setCases(data.items || [])
        setTotalCount(data.total || 0)
        setFailed(false)
      } catch (error) {
        if (requestId !== requestIdRef.current) return
        console.error('Failed to fetch cases:', error)
        // Show the failure rather than an empty list or the previous page's rows.
        setCases([])
        setFailed(true)
      } finally {
        if (requestId === requestIdRef.current) setLoading(false)
      }
    }

    void fetchCases()
  }, [page, retryCount])

  const th = 'px-6 py-3 text-left text-xs font-semibold text-foreground-muted uppercase tracking-wider'

  return (
    <div style={{ minHeight: `calc(100vh - ${CONTENT_TOP_PX}px)` }} className="p-6">
      <div className="flex items-center gap-3 mb-6">
        <h1 className="text-2xl font-semibold text-foreground tracking-tight">{t('cases.list.title')}</h1>
        {totalCount > 0 && (
          <span className="inline-flex items-center rounded-full bg-primary-subtle px-2.5 py-0.5 text-sm font-medium text-primary">
            {totalCount}
          </span>
        )}
      </div>

      <div className="rounded-2xl bg-app-surface shadow-md overflow-hidden">
        <div className="relative min-h-[400px]">
          {loading && (
            <div className="absolute inset-0 bg-app-surface/60 backdrop-blur-[1px] z-10 flex flex-col items-center justify-center gap-2">
              <Spinner size="3" />
              <Text size="2" color="gray">
                {t('cases.list.loading')}
              </Text>
            </div>
          )}
          {failed ? (
            <div className="p-16 flex flex-col items-center gap-4 text-center">
              <Text size="3" color="red">
                {t('cases.list.error')}
              </Text>
              <Button variant="soft" onClick={() => setRetryCount((n) => n + 1)} disabled={loading}>
                {t('cases.list.retry')}
              </Button>
            </div>
          ) : cases.length === 0 ? (
            <div className="p-16 text-center">
              <Text size="3" color="gray">
                {t('cases.list.empty')}
              </Text>
            </div>
          ) : (
            <div className="overflow-x-auto">
              <table className="w-full">
                <thead>
                  <tr className="bg-app-surface-muted">
                    <th className={th}>{t('cases.list.table.id')}</th>
                    <th className={th}>{t('cases.list.table.state')}</th>
                    <th className={th}>{t('cases.list.table.created')}</th>
                  </tr>
                </thead>
                <tbody className="divide-y divide-border/60">
                  {cases.map((c) => (
                    <tr
                      key={c.id}
                      onClick={() => void navigate(`/consignments/${c.id}`)}
                      className="hover:bg-primary-subtle cursor-pointer transition-colors"
                    >
                      <td className="px-6 py-4 whitespace-nowrap">
                        {/* The row click is a mouse shortcut; this link is the keyboard path. */}
                        <Link
                          to={`/consignments/${c.id}`}
                          onClick={(e) => e.stopPropagation()}
                          className="inline-block rounded focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary"
                        >
                          {c.name ? (
                            <div className="flex flex-col">
                              <Text size="2" weight="bold" className="text-info-strong">
                                {c.name}
                              </Text>
                              <Text size="1" color="gray" className="font-mono mt-0.5">
                                {c.id}
                              </Text>
                            </div>
                          ) : (
                            <Text size="2" weight="medium" className="text-info-strong font-mono">
                              {c.id}
                            </Text>
                          )}
                        </Link>
                      </td>
                      <td className="px-6 py-4 whitespace-nowrap">
                        <Badge size="1" color={getStateColor(c.state)}>
                          {formatState(c.state)}
                        </Badge>
                      </td>
                      <td className="px-6 py-4 whitespace-nowrap">
                        <Text size="2" color="gray">
                          {c.createdAt ? formatDateTime(c.createdAt) : '-'}
                        </Text>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </div>
        <div className="bg-app-surface/60">
          <PaginationControl
            currentPage={page + 1}
            totalPages={Math.ceil(totalCount / limit)}
            onPageChange={(p) => setPage(p - 1)}
            hasNext={(page + 1) * limit < totalCount}
            hasPrev={page > 0}
            totalCount={totalCount}
          />
        </div>
      </div>
    </div>
  )
}
