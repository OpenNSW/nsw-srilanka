import { useState, useEffect, useRef, type ChangeEvent } from 'react'
import { useDebounce } from '@/hooks/useDebounce.ts'
import { useNavigate } from 'react-router-dom'
import { Badge, Button, Select, Spinner, Text, TextField } from '@radix-ui/themes'
import { MagnifyingGlassIcon, PlusIcon } from '@radix-ui/react-icons'
import { useTranslation } from 'react-i18next'
import type { ConsignmentSummary, TradeFlow, ConsignmentState } from '@/features/consignment/types.ts'
import { createConsignment, getAllConsignments } from '@/features/consignment/service.ts'
import { getStateColor, formatState, formatDateTime } from '@/features/consignment/utils.ts'
import { PaginationControl } from '@/components/common/PaginationControl.tsx'
import { LoadError } from '@/components/common/LoadError.tsx'
import { LinkRow, RowLink } from '@/components/common/LinkRow.tsx'
import { ROUTES } from '@/constants'

export function ConsignmentScreen() {
  const navigate = useNavigate()
  const { t } = useTranslation()
  const [consignments, setConsignments] = useState<ConsignmentSummary[]>([])

  const [totalCount, setTotalCount] = useState(0)
  const [loading, setLoading] = useState(true)
  const [loadFailed, setLoadFailed] = useState(false)
  const [retryCount, setRetryCount] = useState(0)
  const [page, setPage] = useState(0)
  const limit = 50

  const [searchQuery, setSearchQuery] = useState('')
  const debouncedSearchQuery = useDebounce(searchQuery, 400)
  const [stateFilter, setStateFilter] = useState<string>('all')
  const [tradeFlowFilter, setTradeFlowFilter] = useState<string>('all')

  const [creating, setCreating] = useState(false)
  const [createError, setCreateError] = useState(false)

  const listRequestIdRef = useRef(0)

  const [prevDebouncedSearchQuery, setPrevDebouncedSearchQuery] = useState(debouncedSearchQuery)
  if (debouncedSearchQuery !== prevDebouncedSearchQuery) {
    setPrevDebouncedSearchQuery(debouncedSearchQuery)
    setPage(0)
  }

  const handleCreateConsignment = async () => {
    setCreating(true)
    setCreateError(false)
    try {
      const response = await createConsignment()
      void navigate(ROUTES.CONSIGNMENT_DETAIL(response.id))
    } catch (error) {
      console.error('Failed to create consignment:', error)
      setCreateError(true)
    } finally {
      setCreating(false)
    }
  }

  useEffect(() => {
    async function fetchConsignments() {
      const requestId = ++listRequestIdRef.current
      setLoading(true)
      try {
        const data = await getAllConsignments(
          page * limit,
          limit,
          stateFilter as ConsignmentState | 'all',
          tradeFlowFilter as TradeFlow | 'all',
          debouncedSearchQuery,
        )
        if (requestId !== listRequestIdRef.current) {
          return
        }
        setConsignments(data.items || [])
        setTotalCount(data.total || 0)
        setLoadFailed(false)
      } catch (error) {
        if (requestId !== listRequestIdRef.current) {
          return
        }
        console.error('Failed to fetch consignments:', error)
        // A failure is not an empty list. Clear any rows from the previous page.
        setConsignments([])
        setTotalCount(0)
        setLoadFailed(true)
      } finally {
        if (requestId === listRequestIdRef.current) {
          setLoading(false)
        }
      }
    }

    void fetchConsignments()
  }, [page, stateFilter, tradeFlowFilter, debouncedSearchQuery, retryCount])

  return (
    <div style={{ minHeight: 'calc(100vh - var(--app-top-offset))' }} className="p-6">
      <div className="flex flex-wrap items-center justify-between gap-4 mb-6">
        <div className="flex items-center gap-3">
          <h1 className="text-2xl font-semibold text-foreground tracking-tight">{t('consignments.list.title')}</h1>
          {totalCount > 0 && (
            <span className="inline-flex items-center rounded-full bg-primary-subtle px-2.5 py-0.5 text-sm font-medium text-primary">
              {totalCount}
            </span>
          )}
        </div>
        <div className="flex flex-col items-end gap-2">
          <Button onClick={() => void handleCreateConsignment()} disabled={creating} loading={creating}>
            <PlusIcon />
            {creating ? t('consignments.list.creating') : t('consignments.list.create')}
          </Button>
          {createError && (
            <Text size="2" color="red">
              {t('consignments.list.error.createFailed')}
            </Text>
          )}
        </div>
      </div>

      <div className="rounded-2xl bg-app-surface shadow-md overflow-hidden">
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
            <div className="flex gap-3">
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
              <Select.Root
                value={tradeFlowFilter}
                onValueChange={(val: string) => {
                  setTradeFlowFilter(val)
                  setPage(0)
                }}
              >
                <Select.Trigger placeholder={t('consignments.list.filter.tradeFlowPlaceholder')} />
                <Select.Content>
                  <Select.Item value="all">{t('consignments.list.filter.allTypes')}</Select.Item>
                  <Select.Item value="IMPORT">{t('consignments.list.filter.import')}</Select.Item>
                  <Select.Item value="EXPORT">{t('consignments.list.filter.export')}</Select.Item>
                </Select.Content>
              </Select.Root>
            </div>
          </div>
        </div>

        <div className="relative min-h-[400px]">
          {loading && (
            <div className="absolute inset-0 bg-app-surface/60 backdrop-blur-[1px] z-10 flex flex-col items-center justify-center gap-2">
              <Spinner size="3" />
              <Text size="2" color="gray">
                {t('consignments.list.loading')}
              </Text>
            </div>
          )}
          {loadFailed ? (
            <div className="p-16">
              <LoadError
                message={t('consignments.list.error.loadFailed')}
                onRetry={() => setRetryCount((n) => n + 1)}
              />
            </div>
          ) : consignments.length === 0 ? (
            <div className="p-16 text-center">
              <Text size="3" color="gray">
                {debouncedSearchQuery || stateFilter !== 'all' || tradeFlowFilter !== 'all'
                  ? t('consignments.list.empty.filtered')
                  : t('consignments.list.empty.none')}
              </Text>
            </div>
          ) : (
            <div className="overflow-x-auto">
              <table className="w-full">
                <thead>
                  <tr className="bg-app-surface-muted">
                    <th className="px-6 py-3 text-left text-xs font-semibold text-foreground-muted uppercase tracking-wider">
                      {t('consignments.list.table.id')}
                    </th>
                    <th className="px-6 py-3 text-left text-xs font-semibold text-foreground-muted uppercase tracking-wider">
                      {t('consignments.list.table.tradeFlow')}
                    </th>
                    <th className="px-6 py-3 text-left text-xs font-semibold text-foreground-muted uppercase tracking-wider">
                      {t('consignments.list.table.state')}
                    </th>
                    <th className="px-6 py-3 text-left text-xs font-semibold text-foreground-muted uppercase tracking-wider">
                      {t('consignments.list.table.created')}
                    </th>
                  </tr>
                </thead>
                <tbody className="divide-y divide-border/60">
                  {consignments.map((consignment) => (
                    <LinkRow key={consignment.id} to={ROUTES.CONSIGNMENT_DETAIL(consignment.id)}>
                      <td className="px-6 py-4 whitespace-nowrap">
                        <RowLink to={ROUTES.CONSIGNMENT_DETAIL(consignment.id)}>
                          {consignment.name ? (
                            <div className="flex flex-col">
                              <Text size="2" weight="bold" className="text-info-strong">
                                {consignment.name}
                              </Text>
                              <Text size="1" color="gray" className="font-mono mt-0.5">
                                {consignment.id}
                              </Text>
                            </div>
                          ) : (
                            <Text size="2" weight="medium" className="text-info-strong font-mono">
                              {consignment.id}
                            </Text>
                          )}
                        </RowLink>
                      </td>
                      <td className="px-6 py-4 whitespace-nowrap">
                        <Badge size="1" color={consignment.flow === 'IMPORT' ? 'blue' : 'green'} variant="soft">
                          {consignment.flow === 'IMPORT'
                            ? t('consignments.tradeFlow.import')
                            : t('consignments.tradeFlow.export')}
                        </Badge>
                      </td>
                      <td className="px-6 py-4 whitespace-nowrap">
                        <Badge size="1" color={getStateColor(consignment.state)}>
                          {formatState(consignment.state)}
                        </Badge>
                      </td>
                      <td className="px-6 py-4 whitespace-nowrap">
                        <Text size="2" color="gray">
                          {consignment.createdAt ? formatDateTime(consignment.createdAt) : '-'}
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
      </div>
    </div>
  )
}
