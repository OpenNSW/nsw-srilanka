import { useCallback, useEffect, useState } from 'react'
import { useParams, useNavigate } from 'react-router-dom'
import { Button, Badge, Spinner, Text, Progress, Flex, IconButton, Tooltip as RadixTooltip } from '@radix-ui/themes'
import { ArrowLeftIcon, ListBulletIcon, ViewGridIcon, InfoCircledIcon, CheckCircledIcon, ClockIcon, MagnifyingGlassIcon } from '@radix-ui/react-icons'
import { WorkflowViewer, ActionListView } from '../components/WorkflowViewer'
import type { ConsignmentDetail } from "../services/types/consignment.ts"
import { getConsignment, initializeConsignment } from "../services/consignment.ts"
import { useApi } from '../services/ApiContext'
import { useRole } from '../services/RoleContext'
import { getStateColor, formatState, formatDateTime } from '../utils/consignmentUtils'
import { HSCodePicker } from '../components/HSCodePicker'
import type { HSCode } from '../services/types/hsCode'

export function ConsignmentDetailScreen() {
  const { consignmentId } = useParams<{ consignmentId: string }>()
  const navigate = useNavigate()
  const api = useApi()
  const [consignment, setConsignment] = useState<ConsignmentDetail | null>(null)
  const [loading, setLoading] = useState(true)
  const [refreshing, setRefreshing] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [hsPickerOpen, setHsPickerOpen] = useState(false)
  const [initializing, setInitializing] = useState(false)
  const [viewMode, setViewMode] = useState<'list' | 'graph'>('list')

  const { role } = useRole()

  const fetchConsignment = useCallback(async () => {
    if (!consignmentId) {
      setError('Consignment ID is required')
      setLoading(false)
      return
    }

    setLoading(true)
    setError(null)
    try {
      const result = await getConsignment(consignmentId, api)
      if (result) {
        setConsignment(result)
      } else {
        setError('Consignment not found')
      }
    } catch (err) {
      console.error('Failed to fetch consignment:', err)
      setError('Failed to load consignment')
    } finally {
      setLoading(false)
      setRefreshing(false)
    }
  }, [api, consignmentId])

  const handleRefresh = () => {
    setRefreshing(true)
    fetchConsignment()
  }

  useEffect(() => {
    fetchConsignment()
  }, [fetchConsignment])

  if (loading) {
    const isProcessing = !consignment // If we don't have consignment data yet, we're in initial load
    return (
      <div className="p-6">
        <div className="flex items-center justify-center py-12">
          <Spinner size="3" />
          <Text size="3" color="gray" className="ml-3">
            {isProcessing ? 'Processing your submission...' : 'Loading consignment...'}
          </Text>
        </div>
      </div>
    )
  }

  if (error || !consignment) {
    return (
      <div className="p-6">
        <div className="mb-6">
          <Button variant="ghost" color="gray" onClick={() => navigate('/consignments')}>
            <ArrowLeftIcon />
            Back
          </Button>
        </div>
        <div className="bg-white rounded-lg shadow p-8 text-center">
          <Text size="5" color="red" weight="medium" className="block mb-2">
            {error || 'Consignment not found'}
          </Text>
          <Text size="2" color="gray" className="block mb-6">
            {error === 'Failed to load consignment'
              ? 'There was a problem loading the consignment details. Please try again.'
              : 'The consignment you\'re looking for doesn\'t exist or you don\'t have access to it.'}
          </Text>
          <div className="flex gap-3 justify-center">
            <Button variant="soft" onClick={() => navigate('/consignments')}>
              <ArrowLeftIcon />
              Back to Consignments
            </Button>
            {error === 'Failed to load consignment' && (
              <Button onClick={fetchConsignment}>
                Try Again
              </Button>
            )}
          </div>
        </div>
      </div>
    )
  }

  const item = consignment.items?.[0]
  const workflowNodes = consignment.workflowNodes || []
  const completedSteps = workflowNodes.filter(n => n.state === 'COMPLETED').length
  const totalSteps = workflowNodes.length
  const progressPercentage = totalSteps > 0 ? (completedSteps / totalSteps) * 100 : 0
  const isChaView = role === 'cha'

  const handleSelectHSCode = async (hsCode: HSCode) => {
    if (!consignmentId) return
    setInitializing(true)
    try {
      await initializeConsignment(consignmentId, [hsCode.id], api)
      setHsPickerOpen(false)
      await fetchConsignment()
    } catch (e) {
      console.error('Failed to initialize consignment:', e)
      // keep it minimal: reuse existing error area
      setError('Failed to initialize consignment')
    } finally {
      setInitializing(false)
    }
  }

  return (
    <div className="p-4 md:p-6 h-[calc(100vh-64px)] flex flex-col">
      <div className="mb-4 md:mb-6">
        <Button variant="ghost" color="gray" onClick={() => navigate('/consignments')} aria-label="Back to consignments list">
          <ArrowLeftIcon />
          Back
        </Button>
      </div>

      <div className="bg-white rounded-lg shadow flex flex-col flex-1 min-h-0 relative">
        {refreshing && (
          <div className="absolute inset-0 bg-white/80 backdrop-blur-sm z-20 flex items-center justify-center rounded-lg">
            <div className="flex items-center gap-3 bg-white px-6 py-4 rounded-lg shadow-lg">
              <Spinner size="3" />
              <Text size="3" weight="medium" color="gray">Refreshing...</Text>
            </div>
          </div>
        )}
        <div className="p-4 border-b border-gray-200">
          <div className="flex items-center justify-between">
            <div>
              <h1 className="text-xl font-semibold text-gray-900">
                Consignment
              </h1>
              <p className="text-xs text-gray-500 font-mono">
                {consignment.id}
              </p>
              <p className="text-xs text-gray-500">
                {formatDateTime(consignment.createdAt)}
              </p>
            </div>
            <div className="flex flex-col items-end gap-1.5">
              <Badge size="2" color={getStateColor(consignment.state)}>
                {formatState(consignment.state)}
              </Badge>
              <Badge size="1" color={consignment.flow === 'IMPORT' ? 'blue' : 'green'} variant="soft">
                {consignment.flow}
              </Badge>
            </div>
          </div>
        </div>

        <div className="px-4 py-3 border-b border-gray-200 bg-gray-50/30">
          <div className="grid grid-cols-1 lg:grid-cols-2 gap-4">
            <div>
              <h3 className="text-xs font-medium text-gray-500 mb-1">Item Details</h3>
              <p className="text-sm font-medium text-gray-900">{item?.hsCode?.hsCode || '-'}</p>
              <p className="text-xs text-gray-600 line-clamp-1">{item?.hsCode?.description || '-'}</p>
            </div>
            <div>
              <h3 className="text-xs font-medium text-gray-500 mb-1">Workflow Progress</h3>
              <div className="flex items-center gap-2 mb-1">
                <Progress
                  value={progressPercentage}
                  className="flex-1"
                  size="2"
                  color={progressPercentage === 100 ? 'green' : progressPercentage > 0 ? 'blue' : 'gray'}
                />
                <Text size="1" weight="medium" className="text-gray-700 min-w-[3rem] text-right">
                  {completedSteps}/{totalSteps}
                </Text>
              </div>
              <Text size="1" color="gray">
                {progressPercentage === 100
                  ? 'All steps completed'
                  : `${Math.round(progressPercentage)}% complete`}
              </Text>
            </div>
          </div>
        </div>

        <div className="p-4 flex-1 flex flex-col min-h-0">
          <Flex align="center" justify="between" mb="3">
            <h3 className="text-xs font-bold text-gray-400 uppercase tracking-widest">
              Workflow Process
            </h3>
            
            <Flex gap="1" className="bg-gray-100 p-1 rounded-lg border border-gray-200 shadow-sm">
              <RadixTooltip content="List View (Action Oriented)">
                <IconButton 
                  variant={viewMode === 'list' ? 'solid' : 'soft'} 
                  color={viewMode === 'list' ? 'blue' : 'gray'}
                  highContrast={viewMode !== 'list'}
                  size="2"
                  onClick={() => setViewMode('list')}
                  className="cursor-pointer transition-all duration-200"
                >
                  <ListBulletIcon width="18" height="18" />
                </IconButton>
              </RadixTooltip>
              <RadixTooltip content="Workflow Graph (Visualizer)">
                <IconButton 
                  variant={viewMode === 'graph' ? 'solid' : 'soft'} 
                  color={viewMode === 'graph' ? 'blue' : 'gray'}
                  highContrast={viewMode !== 'graph'}
                  size="2"
                  onClick={() => setViewMode('graph')}
                  className="cursor-pointer transition-all duration-200"
                >
                  <ViewGridIcon width="18" height="18" />
                </IconButton>
              </RadixTooltip>
            </Flex>
          </Flex>

          {workflowNodes.length > 0 ? (
            <div className="flex-1 min-h-0 bg-gray-50/50 rounded-xl border border-gray-200/50 p-2 sm:p-4 shadow-inner flex flex-col overflow-hidden">
              {viewMode === 'list' ? (
                <ActionListView 
                  className="flex-1 min-h-0"
                  steps={workflowNodes} 
                  consignmentId={consignmentId!} 
                  onRefresh={handleRefresh} 
                  refreshing={refreshing} 
                  consignmentState={consignment.state}
                />
              ) : (
                <WorkflowViewer 
                  className="h-full border-0 bg-transparent" 
                  steps={workflowNodes} 
                  onRefresh={handleRefresh} 
                  refreshing={refreshing} 
                />
              )}
            </div>
          ) : consignment.state === 'INITIALIZED' ? (
            <div 
              className={`flex-1 flex items-center justify-center bg-gray-50/50 rounded-xl border border-dashed transition-all duration-200 
                ${isChaView 
                  ? 'border-blue-300 hover:border-blue-500 hover:bg-blue-50/30 cursor-pointer group shadow-sm hover:shadow-md' 
                  : 'border-gray-300'}`}
              onClick={isChaView && !initializing ? () => setHsPickerOpen(true) : undefined}
            >
              <div className="text-center max-w-md p-6">
                {isChaView ? (
                  <>
                    <div className="mb-4 flex justify-center group-hover:scale-110 transition-transform duration-200">
                       <div className="p-3 bg-blue-50 group-hover:bg-blue-100 rounded-full transition-colors">
                         <InfoCircledIcon width="32" height="32" className="text-blue-500" />
                       </div>
                    </div>
                    <Text size="4" weight="bold" className="block mb-2 text-gray-700 group-hover:text-blue-600 transition-colors">
                      Initialize Workflow
                    </Text>
                    <Text size="2" color="gray" className="block mb-6">
                      To begin the consignment process, you must first select the appropriate HS Code for this consignment.
                    </Text>
                    <Flex align="center" justify="center" gap="2" className="text-blue-600 font-semibold group-hover:text-blue-700 transition-colors">
                      {initializing ? (
                        <Spinner size="1" />
                      ) : (
                        <div className="bg-blue-50 text-blue-600 p-1.5 rounded-full group-hover:bg-blue-100 transition-colors shadow-sm">
                          <MagnifyingGlassIcon width="16" height="16" />
                        </div>
                      )}
                      <Text size="3" className="group-hover:underline decoration-2 underline-offset-4">
                        {initializing ? 'Initializing...' : 'Select HS Code'}
                      </Text>
                    </Flex>
                  </>
                ) : (
                  <>
                    <div className="mb-4 flex justify-center">
                       <div className="p-3 bg-amber-50 rounded-full">
                         <ClockIcon width="32" height="32" className="text-amber-500" />
                       </div>
                    </div>
                    <Text size="4" color="gray" weight="bold" className="block mb-2">
                      Awaiting HS Code Selection
                    </Text>
                    <Text size="2" color="gray" className="block">
                      Your Customs House Agent (CHA) needs to select the HS Code for this consignment before the workflow steps can be generated.
                    </Text>
                  </>
                )}
              </div>
            </div>
          ) : (
            <div className="flex-1 flex items-center justify-center">
              <div className="text-center">
                <Text size="4" color="gray" weight="medium" className="block mb-2">
                  No Workflow Steps
                </Text>
                <Text size="2" color="gray">
                  This consignment doesn't have any workflow steps configured.
                </Text>
              </div>
            </div>
          )}
        </div>

        <div className="px-4 py-3 border-t border-gray-200 bg-gradient-to-r from-blue-50/50 to-indigo-50/50">
          <Flex align="center" gap="2" mb="1">
            <div className="w-1.5 h-3 bg-blue-500 rounded-full" />
            <h3 className="text-xs font-bold text-gray-700">
              Next Steps
            </h3>
          </Flex>
          {consignment.state === 'INITIALIZED' ? (
            <Text size="1" color="gray" className="flex items-center gap-1.5">
              <InfoCircledIcon className="text-blue-500" />
              <span className="font-medium text-gray-900">Current Bottleneck:</span> 
              {isChaView 
                ? "Waiting for you to select an HS Code to initialize the workflow process." 
                : "Waiting for CHA to select an HS Code to initialize the workflow process."}
            </Text>
          ) : workflowNodes.length === 0 ? (
            <Text size="1" color="gray">
              No actions required at this time.
            </Text>
          ) : workflowNodes.some(n => n.state === 'READY') ? (
            <Text size="1" color="gray" className="flex items-center gap-1.5">
              <InfoCircledIcon className="text-blue-500" />
              <span className="font-medium text-gray-900">Action required:</span> 
              Proceed with tasks marked as <Badge size="1" color="blue" variant="soft">Ready</Badge> in the list above.
            </Text>
          ) : workflowNodes.every(n => n.state === 'COMPLETED') ? (
            <Text size="1" color="green" weight="medium" className="flex items-center gap-1.5">
              <CheckCircledIcon />
              All steps have been completed. Your consignment is ready.
            </Text>
          ) : (
            <Text size="1" color="gray" className="flex items-center gap-1.5">
              <ClockIcon />
              Waiting for dependent steps to be completed before you can proceed.
            </Text>
          )}
        </div>
      </div>

      {/* Reuse existing HS code modal, but skip trade-flow step (flow is already known) */}
      <HSCodePicker
        open={hsPickerOpen}
        onOpenChange={setHsPickerOpen}
        fixedTradeFlow={consignment.flow}
        title="Select HS Code"
        confirmText="Start Workflow"
        cancelText="Cancel"
        isCreating={initializing}
        onSelect={(hsCode) => handleSelectHSCode(hsCode)}
      />
    </div>
  )
}