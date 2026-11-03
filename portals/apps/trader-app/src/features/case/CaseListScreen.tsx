import { useState } from 'react'
import { Badge, Text } from '@radix-ui/themes'
import { useTranslation } from 'react-i18next'
import { getStateColor, formatState, formatDateTime } from '@/features/consignment/utils.ts'
import type { CaseSummary } from './types'
import { getCases } from './service'
import { ROUTES } from '@/constants'
import { useQuery } from '@/hooks/useQuery'
import { LinkRow, LoadingOverlay, PageHeader, PaginationControl, RowLink, Surface, PageState } from '@portals/ui'

// The officer's list of cases. A trimmed ConsignmentScreen: no create button, no
// trade-flow filter, no role switch — an officer sees every case.
export function CaseListScreen() {
  const { t } = useTranslation()
  const [page, setPage] = useState(0)
  const limit = 50
  const { data, error, isFetching, refetch } = useQuery(
    ['cases', page],
    (signal) => getCases(page * limit, limit, signal),
    { keepPreviousData: true },
  )
  // Show the failure rather than an empty list or the previous page's rows.
  const cases: CaseSummary[] = error ? [] : (data?.items ?? [])
  const totalCount = data?.total ?? 0

  const th = 'px-6 py-3 text-left text-xs font-semibold text-foreground-muted uppercase tracking-wider'

  return (
    <div style={{ minHeight: 'calc(100vh - var(--app-top-offset))' }} className="p-6">
      <PageHeader title={t('cases.list.title')} count={totalCount} />

      <Surface className="overflow-hidden">
        <div className="relative min-h-[400px]">
          {isFetching && <LoadingOverlay message={t('cases.list.loading')} />}
          {error ? (
            <PageState variant="error" title={t('cases.list.error')} onRetry={() => void refetch()} />
          ) : cases.length === 0 ? (
            <PageState variant="empty" title={t('cases.list.empty')} />
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
                    <LinkRow key={c.id} to={ROUTES.CONSIGNMENT_DETAIL(c.id)}>
                      <td className="px-6 py-4 whitespace-nowrap">
                        <RowLink to={ROUTES.CONSIGNMENT_DETAIL(c.id)}>
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
                        </RowLink>
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
                    </LinkRow>
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
      </Surface>
    </div>
  )
}
