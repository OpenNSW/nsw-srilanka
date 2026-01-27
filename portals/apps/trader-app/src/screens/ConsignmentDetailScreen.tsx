import { useEffect, useState } from 'react'
import { useParams, useNavigate, useLocation } from 'react-router-dom'
import { Button, Badge, Spinner, Text } from '@radix-ui/themes'
import { ArrowLeftIcon } from '@radix-ui/react-icons'
import { WorkflowViewer } from '../components/WorkflowViewer'
import type { Consignment } from "../services/types/consignment.ts"
import { getConsignment } from "../services/consignment.ts"
import { getStateColor, formatState } from '../utils/consignmentUtils'

export function ConsignmentDetailScreen() {
  const { consignmentId } = useParams<{ consignmentId: string }>()
  const navigate = useNavigate()
  const location = useLocation()
  const [consignment, setConsignment] = useState<Consignment | null>(null)
  const [loading, setLoading] = useState(true)
  const [refreshing, setRefreshing] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const fetchConsignment = async () => {
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
  }

  const handleRefresh = () => {
    setRefreshing(true)
    fetchConsignment()
  }

  useEffect(() => {
    // Check if we just submitted a form
    const state = location.state as { justSubmitted?: boolean } | null
    if (state?.justSubmitted) {
      // Clear the navigation state to prevent re-triggering on refresh
      navigate(location.pathname, { replace: true, state: {} })

      // Show loading state and wait 5 seconds before fetching
      setLoading(true)
      const timer = setTimeout(() => {
        fetchConsignment()
      }, 5000)

      return () => clearTimeout(timer)
    } else {
      // Normal fetch without delay
      fetchConsignment()
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
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
        <div className="bg-white rounded-lg shadow p-6 text-center">
          <Text size="4" color="red" weight="medium">
            {error || 'Consignment not found'}
          </Text>
          <div className="mt-4">
            <Button variant="soft" onClick={() => navigate('/consignments')}>
              <ArrowLeftIcon />
              Back to Consignments
            </Button>
          </div>
        </div>
      </div>
    )
  }

  const item = consignment.items[0]
  const steps = item?.steps || []
  const completedSteps = steps.filter(s => s.status === 'COMPLETED').length
  const totalSteps = steps.length

  return (
    <div className="p-6">
      <div className="mb-6">
        <Button variant="ghost" color="gray" onClick={() => navigate('/consignments')}>
          <ArrowLeftIcon />
          Back
        </Button>
      </div>

      <div className="bg-white rounded-lg shadow">
        <div className="p-6 border-b border-gray-200">
          <div className="flex items-center justify-between">
            <div>
              <h1 className="text-2xl font-semibold text-gray-900">
                Consignment
              </h1>
              <p className="mt-1 text-sm text-gray-500 font-mono">
                {consignment.id}
              </p>
              <p className="mt-1 text-sm text-gray-500">
                Created on {new Date(consignment.createdAt).toLocaleDateString('en-US', {
                  year: 'numeric',
                  month: 'long',
                  day: 'numeric',
                  hour: '2-digit',
                  minute: '2-digit',
                })}
              </p>
            </div>
            <div className="flex flex-col items-end gap-2">
              <Badge size="2" color={getStateColor(consignment.state)}>
                {formatState(consignment.state)}
              </Badge>
              <Badge size="1" color={consignment.tradeFlow === 'IMPORT' ? 'blue' : 'green'} variant="soft">
                {consignment.tradeFlow}
              </Badge>
            </div>
          </div>
        </div>

        <div className="p-6">
          <div className="grid grid-cols-1 md:grid-cols-3 gap-6">
            <div>
              <h3 className="text-sm font-medium text-gray-500 mb-2">HS Code</h3>
              <p className="text-lg font-medium text-gray-900">{item?.hsCodeID || '-'}</p>
            </div>
            <div>
              <h3 className="text-sm font-medium text-gray-500 mb-2">Trader ID</h3>
              <p className="text-lg font-medium text-gray-900">{consignment.traderId}</p>
            </div>
            <div>
              <h3 className="text-sm font-medium text-gray-500 mb-2">Progress</h3>
              <p className="text-lg font-medium text-gray-900">{completedSteps}/{totalSteps} steps completed</p>
            </div>
          </div>
        </div>

        {steps.length > 0 && (
          <div className="p-6 border-t border-gray-200">
            <h3 className="text-sm font-medium text-gray-500 mb-4">Workflow Process</h3>
            <WorkflowViewer steps={steps} onRefresh={handleRefresh} refreshing={refreshing} />
          </div>
        )}

        <div className="p-6 border-t border-gray-200 bg-gray-50">
          <h3 className="text-sm font-medium text-gray-500 mb-2">Next Steps</h3>
          {steps.some(s => s.status === 'READY') ? (
            <p className="text-sm text-gray-600">
              Click the play button on steps marked as "Ready" to proceed with your consignment.
            </p>
          ) : steps.every(s => s.status === 'COMPLETED') ? (
            <p className="text-sm text-green-600">
              All steps have been completed. Your consignment is ready.
            </p>
          ) : (
            <p className="text-sm text-gray-600">
              Waiting for dependent steps to be completed before you can proceed.
            </p>
          )}
        </div>
      </div>
    </div>
  )
}