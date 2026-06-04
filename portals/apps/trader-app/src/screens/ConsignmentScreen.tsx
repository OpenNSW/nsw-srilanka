import { useState, useEffect, useRef, type ChangeEvent } from 'react'
import { useNavigate } from 'react-router-dom'
import { Badge, Box, Button, Dialog, Flex, IconButton, Select, Spinner, Text, TextField } from '@radix-ui/themes'
import { MagnifyingGlassIcon, PlusIcon, Cross2Icon, InfoCircledIcon } from '@radix-ui/react-icons'
import type { ConsignmentSummary, TradeFlow, ConsignmentState } from '../services/types/consignment.ts'
import { createConsignment, getAllConsignments } from '../services/consignment.ts'
import { getCompanies } from '../services/company.ts'
import { useApi } from '../services/ApiContext'
import { useRole } from '../services/RoleContext'
import { getStateColor, formatState, formatDateTime } from '../utils/consignmentUtils'
import { PaginationControl } from '../components/common/PaginationControl'

import { CHASearch, type CHAOption } from '../components/CHAPicker/CHASearch'

// Local alias (avoid a second themes import just for Text-as)
const RadixText = Text

type NewConsignmentData = {
  flow: TradeFlow | null
  chaCompanyId: string
}

interface NewConsignmentDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  creating: boolean
  onCreate: (data: NewConsignmentData) => Promise<void>
}

function NewConsignmentDialog({ open, onOpenChange, creating, onCreate }: NewConsignmentDialogProps) {
  const api = useApi()
  const [newConsignmentData, setNewConsignmentData] = useState<NewConsignmentData>({
    flow: null,
    chaCompanyId: '',
  })
  const [selectedCHA, setSelectedCHA] = useState<CHAOption | null>(null)
  const [currentCHASearchQuery, setCurrentCHASearchQuery] = useState('')
  const [chaOptions, setChaOptions] = useState<CHAOption[]>([])
  const [chaLoading, setChaLoading] = useState(false)

  useEffect(() => {
    if (!open) {
      setNewConsignmentData({ flow: null, chaCompanyId: '' })
      setSelectedCHA(null)
      setCurrentCHASearchQuery('')
      setChaOptions([])
      setChaLoading(false)
      return
    }

    if (!currentCHASearchQuery.trim()) {
      setChaOptions([])
      setChaLoading(false)
      return
    }

    let cancelled = false
    const handle = setTimeout(() => {
      setChaLoading(true)
      void getCompanies({ hasCha: true, name: currentCHASearchQuery }, api)
        .then((companies) => {
          if (cancelled) return
          setChaOptions(companies.map((c) => ({ id: c.id, name: c.name })))
        })
        .catch((err: unknown) => {
          if (cancelled) return
          console.error('Failed to fetch companies:', err)
          setChaOptions([])
        })
        .finally(() => {
          if (cancelled) return
          setChaLoading(false)
        })
    }, 300)
    return () => {
      cancelled = true
      clearTimeout(handle)
    }
  }, [open, currentCHASearchQuery, api])

  const handleCreate = () => {
    if (newConsignmentData.flow && newConsignmentData.chaCompanyId) {
      void onCreate(newConsignmentData)
    }
  }
  return (
    <Dialog.Root open={open} onOpenChange={onOpenChange}>
      <Dialog.Content
        maxWidth="450px"
        style={{ minHeight: '420px', display: 'flex', flexDirection: 'column' }}
        onInteractOutside={(e: Event) => e.preventDefault()}
      >
        <Flex justify="between" align="start">
          <Box>
            <Dialog.Title>New Consignment</Dialog.Title>
          </Box>
          <Dialog.Close>
            <IconButton variant="ghost" color="gray" size="1">
              <Cross2Icon />
            </IconButton>
          </Dialog.Close>
        </Flex>

        <Box mt="4" />

        <Box style={{ flex: 1 }}>
          <Flex direction="column" gap="5">
            {/* Section 1: Trade Flow */}
            <Box>
              <RadixText size="2" weight="medium" color="gray" as="div" mb="2">
                Select Trade Flow
              </RadixText>
              <Flex gap="3">
                <button
                  onClick={() => {
                    setNewConsignmentData((prev) => ({
                      ...prev,
                      flow: 'IMPORT',
                    }))
                  }}
                  aria-pressed={newConsignmentData.flow === 'IMPORT'}
                  aria-label="Select Import Trade Flow"
                  className={`flex-1 p-4 border-2 rounded-lg transition-all text-left group cursor-pointer ${
                    newConsignmentData.flow === 'IMPORT'
                      ? 'border-info bg-info-subtle'
                      : 'border-border hover:border-info-subtle hover:bg-info-subtle/50'
                  }`}
                >
                  <Flex align="center">
                    <Box>
                      <Flex align="center" gap="2" mb="1">
                        <RadixText
                          size="3"
                          weight="bold"
                          className={`${newConsignmentData.flow === 'IMPORT' ? 'text-info-strong' : 'text-foreground'} block`}
                        >
                          Import
                        </RadixText>
                      </Flex>
                      <RadixText
                        size="1"
                        color="gray"
                        style={{ fontSize: '10px', lineHeight: '1.2' }}
                        className="truncate block"
                      >
                        Bringing goods into the country
                      </RadixText>
                    </Box>
                  </Flex>
                </button>
                <button
                  onClick={() => {
                    setNewConsignmentData((prev) => ({
                      ...prev,
                      flow: 'EXPORT',
                    }))
                  }}
                  aria-pressed={newConsignmentData.flow === 'EXPORT'}
                  aria-label="Select Export Trade Flow"
                  className={`flex-1 p-4 border-2 rounded-lg transition-all text-left group cursor-pointer ${
                    newConsignmentData.flow === 'EXPORT'
                      ? 'border-success bg-success-subtle'
                      : 'border-border hover:border-success-subtle hover:bg-success-subtle/50'
                  }`}
                >
                  <Flex align="center">
                    <Box>
                      <Flex align="center" gap="2" mb="1">
                        <RadixText
                          size="3"
                          weight="bold"
                          className={`${newConsignmentData.flow === 'EXPORT' ? 'text-success-strong' : 'text-foreground'} block`}
                        >
                          Export
                        </RadixText>
                      </Flex>
                      <RadixText
                        size="1"
                        color="gray"
                        style={{ fontSize: '10px', lineHeight: '1.2' }}
                        className="truncate block"
                      >
                        Sending goods out of the country
                      </RadixText>
                    </Box>
                  </Flex>
                </button>
              </Flex>
            </Box>

            {/* Section 2: CHA Selection */}
            <Box>
              <RadixText size="2" weight="medium" color="gray" as="div" mb="2">
                Select CHA
              </RadixText>
              <CHASearch
                options={chaOptions}
                value={selectedCHA}
                searchQuery={currentCHASearchQuery}
                onChange={(company) => {
                  setSelectedCHA(company)
                  setNewConsignmentData((prev) => ({ ...prev, chaCompanyId: company?.id ?? '' }))
                }}
                onSearchQueryChange={setCurrentCHASearchQuery}
                loading={chaLoading}
              />
              {!newConsignmentData.chaCompanyId && currentCHASearchQuery.trim().length > 0 && (
                <Flex align="center" gap="1" mt="2">
                  <InfoCircledIcon color="red" />
                  <Text size="1" color="red">
                    Please select a CHA company from the list.
                  </Text>
                </Flex>
              )}
            </Box>
          </Flex>
        </Box>

        <Flex gap="3" justify="end" mt="4">
          <Dialog.Close>
            <Button variant="soft" color="red" disabled={creating}>
              Cancel
            </Button>
          </Dialog.Close>
          <Button
            onClick={handleCreate}
            disabled={!newConsignmentData.flow || !newConsignmentData.chaCompanyId || creating}
            loading={creating}
          >
            {creating ? 'Creating...' : 'Create'}
          </Button>
        </Flex>
      </Dialog.Content>
    </Dialog.Root>
  )
}

export function ConsignmentScreen() {
  const navigate = useNavigate()
  const api = useApi()
  const [consignments, setConsignments] = useState<ConsignmentSummary[]>([])

  const [totalCount, setTotalCount] = useState(0)
  const [loading, setLoading] = useState(true)
  const [page, setPage] = useState(0)
  const limit = 50

  // Filters
  const [searchQuery, setSearchQuery] = useState('')
  const [stateFilter, setStateFilter] = useState<string>('all')
  const [tradeFlowFilter, setTradeFlowFilter] = useState<string>('all')

  const { role } = useRole()

  // New consignment state
  const [pickerOpen, setPickerOpen] = useState(false)
  const [creating, setCreating] = useState(false)

  const listRequestIdRef = useRef(0)

  const handleNewOpenChange = (open: boolean) => {
    setPickerOpen(open)
  }

  const handleCreateShell = async (data: NewConsignmentData) => {
    if (!data.flow) return
    setCreating(true)
    try {
      const response = await createConsignment(
        {
          flow: data.flow,
          chaCompanyId: data.chaCompanyId,
        },
        api,
      )
      setPickerOpen(false)
      // ensure list refreshes (user will see it in both views)
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
            Loading Consignments...
          </Text>
        </div>
      </div>
    )
  }

  return (
    <div className="p-6">
      <div className="flex items-center justify-between mb-6">
        <h1 className="text-2xl font-semibold text-foreground">
          Consignments{totalCount > 0 && <span className="ml-2 text-lg font-normal text-foreground-subtle">({totalCount})</span>}
        </h1>
        <div className="flex gap-2">
          {role === 'cha' ? null : (
            <Button onClick={() => handleNewOpenChange(true)} disabled={creating}>
              <PlusIcon />
              {creating ? 'Creating...' : 'New Consignment'}
            </Button>
          )}
        </div>
      </div>

      <NewConsignmentDialog
        open={pickerOpen}
        onOpenChange={handleNewOpenChange}
        creating={creating}
        onCreate={handleCreateShell}
      />

      <div className="mb-6">
        <div className="p-4 border-b border-border">
          <div className="flex flex-col md:flex-row gap-4">
            <div className="flex-1">
              <TextField.Root
                size="2"
                placeholder="Search by ID or HS Code..."
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
                <Select.Trigger placeholder="State" />
                <Select.Content>
                  <Select.Item value="all">All States</Select.Item>
                  <Select.Item value="INITIALIZED">Initialized</Select.Item>
                  <Select.Item value="IN_PROGRESS">In Progress</Select.Item>
                  <Select.Item value="FINISHED">Finished</Select.Item>
                  <Select.Item value="FAILED">Failed</Select.Item>
                </Select.Content>
              </Select.Root>
              <Select.Root
                value={tradeFlowFilter}
                onValueChange={(val: string) => {
                  setTradeFlowFilter(val)
                  setPage(0)
                }}
              >
                <Select.Trigger placeholder="Trade Flow" />
                <Select.Content>
                  <Select.Item value="all">All Types</Select.Item>
                  <Select.Item value="IMPORT">Import</Select.Item>
                  <Select.Item value="EXPORT">Export</Select.Item>
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
                  ? 'No consignments yet.'
                  : 'No consignments yet. Click "New Consignment" to create your first one.'
                : 'No consignments match your filters.'}
            </Text>
          </div>
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full">
              <thead>
                <tr className="border-b border-border bg-surface">
                  <th className="px-6 py-3 text-left text-xs font-medium text-foreground-muted uppercase tracking-wider">
                    Consignment ID
                  </th>
                  {/* HS Code Column removed as per request */}
                  <th className="px-6 py-3 text-left text-xs font-medium text-foreground-muted uppercase tracking-wider">
                    Trade Flow
                  </th>
                  <th className="px-6 py-3 text-left text-xs font-medium text-foreground-muted uppercase tracking-wider">
                    State
                  </th>
                  <th className="px-6 py-3 text-left text-xs font-medium text-foreground-muted uppercase tracking-wider">
                    Created
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
                        <Text size="2" weight="medium" className="text-info-strong font-mono">
                          {consignment.id}
                        </Text>
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
