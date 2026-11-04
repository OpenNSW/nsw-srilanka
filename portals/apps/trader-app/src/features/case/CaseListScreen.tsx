import { useState } from 'react'
import { Text } from '@radix-ui/themes'
import { useTranslation } from 'react-i18next'
import { formatDateTime } from '@/features/consignment/utils.ts'
import type { CaseSummary } from './types'
import { getCases } from './service'
import { ConsignmentName } from '@/features/consignment/components/ConsignmentName'
import { ConsignmentCard } from '@/features/consignment/components/ConsignmentCard'
import { ROUTES } from '@/constants'
import { useQuery } from '@/hooks/useQuery'
import { LinkRow, LoadingOverlay, PageHeader, PageState, PaginationControl, RowLink, Surface, Table } from '@portals/ui'
import { ConsignmentStateBadge } from '@/features/consignment/components/ConsignmentStateBadge'

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
  const totalCount = error ? 0 : (data?.total ?? 0)

  return (
    <div style={{ minHeight: 'calc(100vh - var(--app-top-offset))' }} className="px-4 py-6 sm:px-6 lg:px-8">
      <PageHeader title={t('cases.list.title')} count={totalCount} />

      <Surface className="overflow-hidden">
        <div className="relative min-h-[400px]">
          {isFetching && <LoadingOverlay message={t('cases.list.loading')} />}
          {error ? (
            <PageState variant="error" title={t('cases.list.error')} onRetry={() => void refetch()} />
          ) : cases.length === 0 ? (
            <PageState variant="empty" title={t('cases.list.empty')} />
          ) : (
            <>
              <ul className="flex flex-col gap-3 p-4 md:hidden">
                {cases.map((c) => (
                  <ConsignmentCard
                    key={c.id}
                    to={ROUTES.CONSIGNMENT_DETAIL(c.id)}
                    name={c.name}
                    id={c.id}
                    state={c.state}
                    createdAt={c.createdAt}
                  />
                ))}
              </ul>
              <div className="hidden md:block">
                <Table>
                  <Table.Header>
                    <Table.HeaderCell>{t('cases.list.table.id')}</Table.HeaderCell>
                    <Table.HeaderCell>{t('cases.list.table.state')}</Table.HeaderCell>
                    <Table.HeaderCell>{t('cases.list.table.created')}</Table.HeaderCell>
                  </Table.Header>
                  <Table.Body>
                    {cases.map((c) => (
                      <LinkRow key={c.id} to={ROUTES.CONSIGNMENT_DETAIL(c.id)}>
                        <Table.Cell>
                          <RowLink to={ROUTES.CONSIGNMENT_DETAIL(c.id)}>
                            <ConsignmentName name={c.name} id={c.id} />
                          </RowLink>
                        </Table.Cell>
                        <Table.Cell>
                          <ConsignmentStateBadge state={c.state} />
                        </Table.Cell>
                        <Table.Cell>
                          <Text size="2" color="gray">
                            {c.createdAt ? formatDateTime(c.createdAt) : '-'}
                          </Text>
                        </Table.Cell>
                      </LinkRow>
                    ))}
                  </Table.Body>
                </Table>
              </div>
            </>
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
