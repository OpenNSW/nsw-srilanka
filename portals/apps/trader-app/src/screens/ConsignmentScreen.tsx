import { useState, useEffect, useRef, type ChangeEvent } from 'react'
import { useNavigate } from 'react-router-dom'
import { Badge, Box, Button, Dialog, Flex, IconButton, Select, Spinner, Text, TextField } from '@radix-ui/themes'
import { MagnifyingGlassIcon, PlusIcon, Cross2Icon, InfoCircledIcon } from '@radix-ui/react-icons'
import type { ConsignmentSummary, TradeFlow, ConsignmentState, CHA } from '../services/types/consignment.ts'
import { createConsignment, getAllConsignments, getCHAs } from '../services/consignment.ts'
import { useApi } from '../services/ApiContext'
import { useRole } from '../services/RoleContext'
import { getStateColor, formatState, formatDate } from '../utils/consignmentUtils'
import { PaginationControl } from '../components/common/PaginationControl'

import { CHASearch, type CHAOption } from '../components/CHAPicker/CHASearch'

// Local alias (avoid a second themes import just for Text-as)
const RadixText = Text

type NewConsignmentData = {
  flow: TradeFlow | null
  chaId: string
}

interface NewConsignmentDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  chaOptions: CHAOption[]
  creating: boolean
  onCreate: (data: NewConsignmentData) => Promise<void>
}

function NewConsignmentDialog({ open, onOpenChange, chaOptions, creating, onCreate }: NewConsignmentDialogProps) {
  const [newConsignmentData, setNewConsignmentData] = useState<NewConsignmentData>({
    flow: null,
    chaId: '',
  })
  const [currentCHASearchQuery, setCurrentCHASearchQuery] = useState('')

  const handleCreate = () => {
    if (newConsignmentData.flow && newConsignmentData.chaId) {
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
                      ? 'border-blue-500 bg-blue-50'
                      : 'border-gray-200 hover:border-blue-300 hover:bg-blue-50/50'
                  }`}
                >
                  <Flex align="center">
                    <Box>
                      <Flex align="center" gap="2" mb="1">
                        <RadixText
                          size="3"
                          weight="bold"
                          className={`${newConsignmentData.flow === 'IMPORT' ? 'text-blue-700' : 'text-gray-900'} block`}
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
                      ? 'border-green-500 bg-green-50'
                      : 'border-gray-200 hover:border-green-300 hover:bg-green-50/50'
                  }`}
                >
                  <Flex align="center">
                    <Box>
                      <Flex align="center" gap="2" mb="1">
                        <RadixText
                          size="3"
                          weight="bold"
                          className={`${newConsignmentData.flow === 'EXPORT' ? 'text-green-700' : 'text-gray-900'} block`}
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
                value={chaOptions.find((c) => c.id === newConsignmentData.chaId) ?? null}
                searchQuery={currentCHASearchQuery}
                onChange={(cha) => {
                  setNewConsignmentData((prev) => ({ ...prev, chaId: cha?.id ?? '' }))
                }}
                onSearchQueryChange={setCurrentCHASearchQuery}
              />
              {!newConsignmentData.chaId && currentCHASearchQuery.trim().length > 0 && (
                <Flex align="center" gap="1" mt="2">
                  <InfoCircledIcon color="red" />
                  <Text size="1" color="red">
                    Please select a CHA from the list.
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
            disabled={!newConsignmentData.flow || !newConsignmentData.chaId || creating}
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
  const [chaOptions, setChaOptions] = useState<CHAOption[]>([])

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
          chaId: data.chaId,
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
    async function fetchCHAs() {
      try {
        const data = await getCHAs(api)
        const options: CHAOption[] = data.map((c: CHA) => ({ id: c.id, name: c.name }))
        setChaOptions(options)
      } catch (error) {
        console.error('Failed to fetch CHAs:', error)
      }
    }
    void fetchCHAs()
  }, [api])

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
        setTotalCount(data.totalCount || 0)
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
        <h1 className="text-2xl font-semibold text-gray-900">Consignments</h1>
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
        chaOptions={chaOptions}
        creating={creating}
        onCreate={handleCreateShell}
      />

      <div className="grid grid-cols-1 md:grid-cols-3 gap-6 mb-8">
        <div className="bg-white rounded-lg shadow p-6">
          <h3 className="text-sm font-medium text-gray-500">Total Consignments</h3>
          <p className="mt-2 text-3xl font-semibold text-gray-900">{totalCount}</p>
        </div>
      </div>

      <div className="bg-white rounded-lg shadow mb-6">
        <div className="p-4 border-b border-gray-200">
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
                <tr className="border-b border-gray-200 bg-gray-50">
                  <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">
                    Consignment ID
                  </th>
                  {/* HS Code Column removed as per request */}
                  <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">
                    Trade Flow
                  </th>
                  <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">
                    State
                  </th>
                  <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">
                    Steps
                  </th>
                  <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">
                    Created
                  </th>
                </tr>
              </thead>
              <tbody className="divide-y divide-gray-200">
                {filteredConsignments.map((consignment) => {
                  const completedSteps = consignment.completedWorkflowNodeCount || 0
                  const totalSteps = consignment.workflowNodeCount || 0

                  return (
                    <tr
                      key={consignment.id}
                      onClick={() => void navigate(`/consignments/${consignment.id}`)}
                      className="hover:bg-gray-50 cursor-pointer transition-colors"
                    >
                      <td className="px-6 py-4 whitespace-nowrap">
                        <Text size="2" weight="medium" className="text-blue-600 font-mono">
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
                          {completedSteps}/{totalSteps}
                        </Text>
                      </td>
                      <td className="px-6 py-4 whitespace-nowrap">
                        <Text size="2" color="gray">
                          {consignment.createdAt ? formatDate(consignment.createdAt) : '-'}
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
