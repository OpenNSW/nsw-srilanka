import { useState, useEffect, useRef, type ChangeEvent } from 'react'
import { useNavigate } from 'react-router-dom'
import { Badge, Button, Select, Spinner, Text, TextField } from '@radix-ui/themes'
import { MagnifyingGlassIcon, PlusIcon } from '@radix-ui/react-icons'
import { useTranslation } from 'react-i18next'
import type { ConsignmentSummary, TradeFlow, ConsignmentState } from '../services/types/consignment.ts'
import { createConsignment, getAllConsignments } from '../services/consignment.ts'
import { useApi } from '../services/ApiContext'
import { useRole } from '../services/RoleContext'
import { getStateColor, formatState, formatDateTime } from '../utils/consignmentUtils'
import { PaginationControl } from '../components/common/PaginationControl'

export function ConsignmentScreen() {
  const navigate = useNavigate()
  const api = useApi()
  const { t } = useTranslation()
  const [consignments, setConsignments] = useState<ConsignmentSummary[]>([])

  const [totalCount, setTotalCount] = useState(0)
  const [loading, setLoading] = useState(true)
  const [page, setPage] = useState(0)
  const limit = 50

  const [searchQuery, setSearchQuery] = useState('')
  const [stateFilter, setStateFilter] = useState<string>('all')
  const [tradeFlowFilter, setTradeFlowFilter] = useState<string>('all')

  const { role } = useRole()

  const [creating, setCreating] = useState(false)

  const listRequestIdRef = useRef(0)

  const handleCreateConsignment = async () => {
    setCreating(true)
    try {
      const response = await createConsignment(api)
      void navigate(`/consignments/${response.id}`)
    } catch (error) {
      console.error('Failed to create consignment:', error)
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
          role,
          api,
        )
        if (requestId !== listRequestIdRef.current) {
          return
        }
        setConsignments(data.items || [])
        setTotalCount(data.total || 0)
      } catch (error) {
        if (requestId !== listRequestIdRef.current) {
          return
        }
        console.error('Failed to fetch consignments:', error)
      } finally {
        if (requestId === listRequestIdRef.current) {
          setLoading(false)
        }
      }
    }

    void fetchConsignments()
  }, [api, page, stateFilter, tradeFlowFilter, role])

  const filteredConsignments = consignments.filter((c) => {
    const item = c.items?.[0]
    const hsCode = item?.hsCode?.hsCode || ''
    const description = item?.hsCode?.description || ''
    const matchesSearch =
      searchQuery === '' ||
      c.id.toLowerCase().includes(searchQuery.toLowerCase()) ||
      (c.name && c.name.toLowerCase().includes(searchQuery.toLowerCase())) ||
      hsCode.toLowerCase().includes(searchQuery.toLowerCase()) ||
      description.toLowerCase().includes(searchQuery.toLowerCase())

    return matchesSearch
  })

  if (loading) {
    return (
      <div className="p-6">
        <div className="flex items-center justify-center py-12">
          <Spinner size="3" />
          <Text size="3" color="gray" className="ml-3">
            {t('consignments.list.loading')}
          </Text>
        </div>
      </div>
    )
  }

  return (
    <div className="p-6">
      <div className="flex items-center justify-between mb-6">
        <h1 className="text-2xl font-semibold text-foreground">
          {t('consignments.list.title')}
          {totalCount > 0 && <span className="ml-2 text-lg font-normal text-foreground-subtle">({totalCount})</span>}
        </h1>
        <div className="flex gap-2">
          {role === 'cha' ? null : (
            <Button onClick={() => void handleCreateConsignment()} disabled={creating} loading={creating}>
              <PlusIcon />
              {creating ? t('consignments.list.creating') : t('consignments.list.create')}
            </Button>
          )}
        </div>
      </div>

      <div className="mb-6">
        <div className="p-4 border-b border-border">
          <div className="flex flex-col md:flex-row gap-4">
            <div className="flex-1">
              <TextField.Root
                size="2"
                placeholder={t('consignments.list.searchPlaceholder')}
                value={searchQuery}
                onChange={(e: ChangeEvent<HTMLInputElement>) => setSearchQuery(e.target.value)}
              >
                <TextField.Slot>
                  <MagnifyingGlassIcon height="16" width="16" />
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
                  <Select.Item value="INITIALIZED">{t('consignments.list.filter.initialized')}</Select.Item>
                  <Select.Item value="IN_PROGRESS">{t('consignments.list.filter.inProgress')}</Select.Item>
                  <Select.Item value="FINISHED">{t('consignments.list.filter.finished')}</Select.Item>
                  <Select.Item value="FAILED">{t('consignments.list.filter.failed')}</Select.Item>
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

        {filteredConsignments.length === 0 ? (
          <div className="p-12 text-center">
            <Text size="3" color="gray">
              {consignments.length === 0
                ? role === 'cha'
                  ? t('consignments.list.empty.cha')
                  : t('consignments.list.empty.trader')
                : t('consignments.list.empty.filtered')}
            </Text>
          </div>
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full">
              <thead>
                <tr className="border-b border-border bg-surface">
                  <th className="px-6 py-3 text-left text-xs font-medium text-foreground-muted uppercase tracking-wider">
                    {t('consignments.list.table.id')}
                  </th>
                  <th className="px-6 py-3 text-left text-xs font-medium text-foreground-muted uppercase tracking-wider">
                    {t('consignments.list.table.tradeFlow')}
                  </th>
                  <th className="px-6 py-3 text-left text-xs font-medium text-foreground-muted uppercase tracking-wider">
                    {t('consignments.list.table.state')}
                  </th>
                  <th className="px-6 py-3 text-left text-xs font-medium text-foreground-muted uppercase tracking-wider">
                    {t('consignments.list.table.created')}
                  </th>
                </tr>
              </thead>
              <tbody className="divide-y divide-border">
                {filteredConsignments.map((consignment) => {
                  return (
                    <tr
                      key={consignment.id}
                      onClick={() => void navigate(`/consignments/${consignment.id}`)}
                      className="hover:bg-surface cursor-pointer transition-colors"
                    >
                      <td className="px-6 py-4 whitespace-nowrap">
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
                      </td>
                      <td className="px-6 py-4 whitespace-nowrap">
                        <Badge size="1" color={consignment.flow === 'IMPORT' ? 'blue' : 'green'} variant="soft">
                          {consignment.flow}
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
                    </tr>
                  )
                })}
              </tbody>
            </table>
          </div>
        )}
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
  )
}
