import { useState, type ChangeEvent } from 'react'
import { useDebounce } from '@/hooks/useDebounce.ts'
import { useMutation } from '@/hooks/useMutation'
import { useQuery } from '@/hooks/useQuery'
import { useNavigate } from 'react-router-dom'
import { Button, Select, Spinner, Text, TextField } from '@radix-ui/themes'
import { MagnifyingGlassIcon, PlusIcon } from '@radix-ui/react-icons'
import { useTranslation } from 'react-i18next'
import type { ConsignmentState } from '@/features/consignment/types.ts'
import { createConsignment, getAllConsignments } from '@/features/consignment/service.ts'
import { formatDateTime } from '@/features/consignment/utils.ts'
import { ConsignmentName } from '@/features/consignment/components/ConsignmentName'
import { ConsignmentCard } from '@/features/consignment/components/ConsignmentCard'
import { ROUTES } from '@/constants'
import {
  ConfirmDialog,
  LinkRow,
  LoadingOverlay,
  PageHeader,
  PageState,
  PaginationControl,
  RowLink,
  Surface,
  Table,
} from '@portals/ui'
import { ConsignmentStateBadge } from '@/features/consignment/components/ConsignmentStateBadge'
import { TradeFlowBadge } from '@/features/consignment/components/TradeFlowBadge'
import { Can } from '@/components/Can'

export function ConsignmentScreen() {
  const navigate = useNavigate()
  const { t } = useTranslation()
  const [page, setPage] = useState(0)
  const limit = 50

  const [searchQuery, setSearchQuery] = useState('')
  const debouncedSearchQuery = useDebounce(searchQuery, 400)
  const [stateFilter, setStateFilter] = useState<string>('all')

  const [prevDebouncedSearchQuery, setPrevDebouncedSearchQuery] = useState(debouncedSearchQuery)
  if (debouncedSearchQuery !== prevDebouncedSearchQuery) {
    setPrevDebouncedSearchQuery(debouncedSearchQuery)
    setPage(0)
  }

  const {
    data,
    error: loadError,
    isFetching: loading,
    refetch,
  } = useQuery(
    ['consignments', page, stateFilter, debouncedSearchQuery],
    (signal) =>
      getAllConsignments(
        page * limit,
        limit,
        stateFilter as ConsignmentState | 'all',
        // Every trade flow: the list has no trade-flow filter.
        undefined,
        debouncedSearchQuery,
        signal,
      ),
    { keepPreviousData: true },
  )
  // A failure is not an empty list. Show no rows from the previous page.
  const consignments = loadError ? [] : (data?.items ?? [])
  const totalCount = loadError ? 0 : (data?.total ?? 0)

  const { mutate: create, isPending: creating, error: createError } = useMutation(createConsignment)
  // A consignment can't be deleted once created, so the button asks first.
  const [confirmingCreate, setConfirmingCreate] = useState(false)

  // Runs from the confirmation dialog, which stays open, busy, until the
  // create ends. A failure shows under the button.
  const handleCreateConsignment = async () => {
    try {
      const response = await create()
      void navigate(ROUTES.CONSIGNMENT_DETAIL(response.id))
    } catch (error) {
      console.error('Failed to create consignment:', error)
    } finally {
      setConfirmingCreate(false)
    }
  }

  return (
    <div style={{ minHeight: 'calc(100vh - var(--app-top-offset))' }} className="px-4 py-6 sm:px-6 lg:px-8">
      <PageHeader
        title={t('consignments.list.title')}
        count={totalCount}
        actions={
          <Can I="consignment.create">
            <div className="flex flex-col items-end gap-2">
              <Button
                onClick={() => setConfirmingCreate(true)}
                disabled={creating}
                loading={creating}
                // Below Radix's xs breakpoint (520px) the button shows the short
                // label; its name stays the full one.
                aria-label={creating ? undefined : t('consignments.list.create')}
              >
                <PlusIcon />
                {creating ? (
                  t('consignments.list.creating')
                ) : (
                  <>
                    <span className="min-[520px]:hidden">{t('consignments.list.createShort')}</span>
                    <span className="hidden min-[520px]:inline">{t('consignments.list.create')}</span>
                  </>
                )}
              </Button>
              {createError && (
                <Text size="2" color="red">
                  {t('consignments.list.error.createFailed')}
                </Text>
              )}
            </div>
            <ConfirmDialog
              open={confirmingCreate}
              onOpenChange={setConfirmingCreate}
              title={t('consignments.list.confirm.title')}
              description={t('consignments.list.confirm.description')}
              confirmLabel={t('consignments.list.confirm.create')}
              cancelLabel={t('consignments.list.confirm.cancel')}
              onConfirm={() => void handleCreateConsignment()}
              busy={creating}
            />
          </Can>
        }
      />

      <Surface className="overflow-hidden">
        <div className="p-4 bg-app-surface/60">
          <div className="flex flex-col md:flex-row gap-4">
            <div className="flex-1">
              <TextField.Root
                size="2"
                placeholder={t('consignments.list.searchPlaceholder')}
                value={searchQuery}
                onChange={(e: ChangeEvent<HTMLInputElement>) => setSearchQuery(e.target.value)}
              >
                <TextField.Slot>
                  {loading && searchQuery !== '' ? (
                    <Spinner size="1" />
                  ) : (
                    <MagnifyingGlassIcon height="16" width="16" />
                  )}
                </TextField.Slot>
              </TextField.Root>
            </div>
            <div className="grid grid-cols-1 gap-3 sm:flex sm:flex-wrap">
              <Select.Root
                value={stateFilter}
                onValueChange={(val: string) => {
                  setStateFilter(val)
                  setPage(0)
                }}
              >
                <Select.Trigger placeholder={t('consignments.list.filter.statePlaceholder')} />
                <Select.Content>
                  <Select.Item value="all">{t('consignments.list.filter.allStates')}</Select.Item>
                  <Select.Item value="IN_PROGRESS">{t('consignments.list.filter.inProgress')}</Select.Item>
                  <Select.Item value="FINISHED">{t('consignments.list.filter.finished')}</Select.Item>
                </Select.Content>
              </Select.Root>
            </div>
          </div>
        </div>

        <div className="relative min-h-[400px]">
          {loading && <LoadingOverlay message={t('consignments.list.loading')} />}
          {loadError ? (
            <PageState variant="error" title={t('consignments.list.error.loadFailed')} onRetry={() => void refetch()} />
          ) : consignments.length === 0 ? (
            <PageState
              variant="empty"
              title={
                debouncedSearchQuery || stateFilter !== 'all'
                  ? t('consignments.list.empty.filtered')
                  : t('consignments.list.empty.none')
              }
            />
          ) : (
            <>
              <ul className="flex flex-col gap-3 p-4 md:hidden">
                {consignments.map((consignment) => (
                  <ConsignmentCard
                    key={consignment.id}
                    to={ROUTES.CONSIGNMENT_DETAIL(consignment.id)}
                    name={consignment.name}
                    id={consignment.id}
                    flow={consignment.flow}
                    state={consignment.state}
                    createdAt={consignment.createdAt}
                  />
                ))}
              </ul>
              <div className="hidden md:block">
                <Table>
                  <Table.Header>
                    <Table.HeaderCell>{t('consignments.list.table.id')}</Table.HeaderCell>
                    <Table.HeaderCell>{t('consignments.list.table.tradeFlow')}</Table.HeaderCell>
                    <Table.HeaderCell>{t('consignments.list.table.state')}</Table.HeaderCell>
                    <Table.HeaderCell>{t('consignments.list.table.created')}</Table.HeaderCell>
                  </Table.Header>
                  <Table.Body>
                    {consignments.map((consignment) => (
                      <LinkRow key={consignment.id} to={ROUTES.CONSIGNMENT_DETAIL(consignment.id)}>
                        <Table.Cell>
                          <RowLink to={ROUTES.CONSIGNMENT_DETAIL(consignment.id)}>
                            <ConsignmentName name={consignment.name} id={consignment.id} />
                          </RowLink>
                        </Table.Cell>
                        <Table.Cell>
                          <TradeFlowBadge flow={consignment.flow} />
                        </Table.Cell>
                        <Table.Cell>
                          <ConsignmentStateBadge state={consignment.state} />
                        </Table.Cell>
                        <Table.Cell>
                          <Text size="2" color="gray">
                            {consignment.createdAt ? formatDateTime(consignment.createdAt) : '-'}
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
