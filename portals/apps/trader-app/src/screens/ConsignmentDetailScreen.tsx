import { useCallback, useEffect, useState } from 'react'
import { useParams, useNavigate } from 'react-router-dom'
import { Button, Badge, Spinner, Text, Progress } from '@radix-ui/themes'
import { ArrowLeftIcon } from '@radix-ui/react-icons'
import { WorkflowViewer } from '../components/WorkflowViewer'
import type { ConsignmentDetail } from "../services/types/consignment.ts"
import { getConsignment } from "../services/consignment.ts"
import { getStateColor, formatState, formatDateTime } from '../utils/consignmentUtils'

export function ConsignmentDetailScreen() {
  const { consignmentId } = useParams<{ consignmentId: string }>()
  const navigate = useNavigate()
  const [consignment, setConsignment] = useState<ConsignmentDetail | null>(null)
  const [loading, setLoading] = useState(true)
  const [refreshing, setRefreshing] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const fetchConsignment = useCallback(async () => {
    if (!consignmentId) {
      setError('Consignment ID is required')
      setLoading(false)
      return
    }

    setLoading(true)
    setError(null)
    try {
      const result = await getConsignment(consignmentId)
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
  }, [consignmentId])

  const handleRefresh = () => {
    setRefreshing(true)
    fetchConsignment()
  }

  useEffect(() => {
    fetchConsignment()
  }, [consignmentId])

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

  const item = consignment.items[0]
  const workflowNodes = consignment.workflowNodes || []
  const completedSteps = workflowNodes.filter(n => n.state === 'COMPLETED').length
  const totalSteps = workflowNodes.length
  const progressPercentage = totalSteps > 0 ? (completedSteps / totalSteps) * 100 : 0

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

        <div className="px-4 py-3 border-b border-gray-200">
          <div className="grid grid-cols-1 lg:grid-cols-2 gap-4">
            <div>
              <h3 className="text-xs font-medium text-gray-500 mb-1">Item Details</h3>
              <p className="text-sm font-medium text-gray-900">{item?.hsCode?.hsCode || '-'}</p>
              <p className="text-xs text-gray-600">{item?.hsCode?.description || '-'}</p>
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

        {workflowNodes.length > 0 ? (
          <div className="p-4 flex-1 flex flex-col min-h-0">
            <h3 className="text-xs font-medium text-gray-500 mb-2">Workflow Process</h3>
            <WorkflowViewer className="flex-1 min-h-0" steps={workflowNodes} onRefresh={handleRefresh} refreshing={refreshing} />
          </div>
        ) : (
          <div className="p-4 flex-1 flex items-center justify-center">
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

        <div className="px-4 py-2.5 border-t border-gray-200 bg-gradient-to-r from-blue-50 to-indigo-50">
          <h3 className="text-xs font-medium text-gray-700 mb-1 flex items-center gap-2">
            <span className="inline-block w-1 h-3 bg-blue-500 rounded"></span>
            Next Steps
          </h3>
          {workflowNodes.length === 0 ? (
            <p className="text-xs text-gray-600">
              No actions required at this time.
            </p>
          ) : workflowNodes.some(n => n.state === 'READY') ? (
            <p className="text-xs text-gray-700">
              <span className="font-medium">Action required:</span> Click the play button (▶) on steps marked as "Ready" to proceed with your consignment.
            </p>
          ) : workflowNodes.every(n => n.state === 'COMPLETED') ? (
            <p className="text-xs text-green-700 font-medium">
              ✓ All steps have been completed. Your consignment is ready.
            </p>
          ) : (
            <p className="text-xs text-gray-600">
              Waiting for dependent steps to be completed before you can proceed.
            </p>
          )}
        </div>
      </div>
    </div>
  )
}