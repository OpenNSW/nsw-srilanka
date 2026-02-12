import { useState, useEffect } from 'react'
import { useNavigate } from 'react-router-dom'
import { Button, Text, TextField, Spinner, Select, Badge } from '@radix-ui/themes'
import { MagnifyingGlassIcon, PlusIcon } from '@radix-ui/react-icons'
import { HSCodePicker } from '../components/HSCodePicker'
import type { HSCode } from "../services/types/hsCode.ts"
import type { Consignment, TradeFlow } from "../services/types/consignment.ts"
import { createConsignment, getAllConsignments } from "../services/consignment.ts"
import { getStateColor, formatState, formatDate } from '../utils/consignmentUtils'

export function ConsignmentScreen() {
  const navigate = useNavigate()
  const [consignments, setConsignments] = useState<Consignment[]>([])
  const [loading, setLoading] = useState(true)

  // Filters
  const [searchQuery, setSearchQuery] = useState('')
  const [stateFilter, setStateFilter] = useState<string>('all')
  const [tradeFlowFilter, setTradeFlowFilter] = useState<string>('all')

  // New consignment state
  const [pickerOpen, setPickerOpen] = useState(false)
  const [creating, setCreating] = useState(false)

  useEffect(() => {
    async function fetchConsignments() {
      try {
        const data = await getAllConsignments()
        setConsignments(data)
      } catch (error) {
        console.error('Failed to fetch consignments:', error)
      } finally {
        setLoading(false)
      }
    }

    fetchConsignments()
  }, [])

  const handleSelect = async (hsCode: HSCode, tradeFlow: TradeFlow) => {
    setCreating(true)

    try {
      const response = await createConsignment({
        flow: tradeFlow,
        items: [
          {
            hsCodeId: hsCode.id,
          },
        ],
      })

      setPickerOpen(false)
      navigate(`/consignments/${response.id}`)
    } catch (error) {
      console.error('Failed to create consignment:', error)
    } finally {
      setCreating(false)
    }
  }

  const filteredConsignments = consignments.filter((c) => {
    const item = c.items[0]
    const hsCode = item?.hsCode?.hsCode || ''
    const description = item?.hsCode?.description || ''
    const matchesSearch =
      searchQuery === '' ||
      c.id.toLowerCase().includes(searchQuery.toLowerCase()) ||
      hsCode.toLowerCase().includes(searchQuery.toLowerCase()) ||
      description.toLowerCase().includes(searchQuery.toLowerCase())

    const matchesState = stateFilter === 'all' || c.state === stateFilter
    const matchesTradeFlow = tradeFlowFilter === 'all' || c.flow === tradeFlowFilter

    return matchesSearch && matchesState && matchesTradeFlow
  })

  // Stats
  const totalConsignments = consignments.length
  const inProgressConsignments = consignments.filter(c => c.state === 'IN_PROGRESS').length
  const completedConsignments = consignments.filter(c => c.state === 'FINISHED').length

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
        <h1 className="text-2xl font-semibold text-gray-900">Consignment</h1>
        <div className="flex gap-2">

          <Button onClick={() => setPickerOpen(true)} disabled={creating}>
            <PlusIcon />
            {creating ? 'Creating...' : 'New Consignment'}
          </Button>
        </div>
      </div>

      <div className="grid grid-cols-1 md:grid-cols-3 gap-6 mb-8">
        <div className="bg-white rounded-lg shadow p-6">
          <h3 className="text-sm font-medium text-gray-500">Total Consignments</h3>
          <p className="mt-2 text-3xl font-semibold text-gray-900">{totalConsignments}</p>
        </div>
        <div className="bg-white rounded-lg shadow p-6">
          <h3 className="text-sm font-medium text-gray-500">In Progress</h3>
          <p className="mt-2 text-3xl font-semibold text-gray-900">{inProgressConsignments}</p>
        </div>
        <div className="bg-white rounded-lg shadow p-6">
          <h3 className="text-sm font-medium text-gray-500">Completed</h3>
          <p className="mt-2 text-3xl font-semibold text-gray-900">{completedConsignments}</p>
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
                onChange={(e) => setSearchQuery(e.target.value)}
              >
                <TextField.Slot>
                  <MagnifyingGlassIcon height="16" width="16" />
                </TextField.Slot>
              </TextField.Root>
            </div>
            <div className="flex gap-3">
              <Select.Root value={stateFilter} onValueChange={setStateFilter}>
                <Select.Trigger placeholder="State" />
                <Select.Content>
                  <Select.Item value="all">All States</Select.Item>
                  <Select.Item value="IN_PROGRESS">In Progress</Select.Item>
                  <Select.Item value="FINISHED">Finished</Select.Item>
                  <Select.Item value="REQUIRES_REWORK">Requires Rework</Select.Item>
                </Select.Content>
              </Select.Root>
              <Select.Root value={tradeFlowFilter} onValueChange={setTradeFlowFilter}>
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
                ? 'No consignments yet. Click "New Consignment" to create your first one.'
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
                  const completedSteps = consignment.workflowNodes?.filter(n => n.state === 'COMPLETED').length || 0
                  const totalSteps = consignment.workflowNodes?.length || 0

                  return (
                    <tr
                      key={consignment.id}
                      onClick={() => navigate(`/consignments/${consignment.id}`)}
                      className="hover:bg-gray-50 cursor-pointer transition-colors"
                    >
                      <td className="px-6 py-4 whitespace-nowrap">
                        <Text size="2" weight="medium" className="text-blue-600 font-mono">
                          {consignment.id}
                        </Text>
                      </td>
                      <td className="px-6 py-4 whitespace-nowrap">
                        <Badge
                          size="1"
                          color={consignment.flow === 'IMPORT' ? 'blue' : 'green'}
                          variant="soft"
                        >
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
      </div>

      <HSCodePicker
        open={pickerOpen}
        onOpenChange={setPickerOpen}
        onSelect={handleSelect}
        isCreating={creating}
      />
    </div>
  )
}