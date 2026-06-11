import { useCallback, useEffect, useState } from 'react'
import { useParams, useNavigate } from 'react-router-dom'
import { Button, Badge, Spinner, Text } from '@radix-ui/themes'
import { ArrowLeftIcon, ReloadIcon } from '@radix-ui/react-icons'
import { ActionListView } from '../components/WorkflowViewer'
import type { ConsignmentDetail } from '../services/types/consignment.ts'
import { getConsignment } from '../services/consignment.ts'
import { useApi } from '../services/ApiContext'
import { getStateColor, formatState, formatDateTime } from '../utils/consignmentUtils'

export function ConsignmentDetailScreen() {
  const { consignmentId } = useParams<{ consignmentId: string }>()
  const navigate = useNavigate()
  const api = useApi()
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
        <div className="bg-background rounded-lg shadow p-8 text-center">
          <Text size="5" color="red" weight="medium" className="block mb-2">
            {error || 'Consignment not found'}
          </Text>
          <Text size="2" color="gray" className="block mb-6">
            {error === 'Failed to load consignment'
              ? 'There was a problem loading the consignment details. Please try again.'
              : "The consignment you're looking for doesn't exist or you don't have access to it."}
          </Text>
          <div className="flex gap-3 justify-center">
            <Button variant="soft" onClick={() => navigate('/consignments')}>
              <ArrowLeftIcon />
              Back to Consignments
            </Button>
            {error === 'Failed to load consignment' && <Button onClick={fetchConsignment}>Try Again</Button>}
          </div>
        </div>
      </div>
    )
  }

  const workflowNodes = consignment.workflowNodes || []

  return (
    <div className="p-4 md:p-6 h-[calc(100vh-64px)] flex flex-col">
      {/* Top row: Back + Refresh */}
      <div className="mb-3 flex items-center justify-between">
        <Button
          variant="ghost"
          color="gray"
          onClick={() => navigate('/consignments')}
          aria-label="Back to consignments list"
        >
          <ArrowLeftIcon />
          Back
        </Button>
        <Button
          variant="soft"
          color="blue"
          size="2"
          onClick={handleRefresh}
          disabled={refreshing}
          className="cursor-pointer"
        >
          <ReloadIcon className={refreshing ? 'animate-spin' : ''} />
          Refresh
        </Button>
      </div>

      {/* Title row */}
      <div className="mb-3 mt-2 flex items-center gap-3">
        <h1 className="text-xl font-semibold text-foreground">Consignment View</h1>
        <Badge size="2" color={getStateColor(consignment.state)}>
          {formatState(consignment.state)}
        </Badge>
        <Badge size="1" color={consignment.flow === 'IMPORT' ? 'blue' : 'green'} variant="soft">
          {consignment.flow}
        </Badge>
      </div>

      {/* ID + date row */}
      <div className="mb-4 md:mb-6 flex items-start gap-10">
        <div>
          <p className="text-xs font-semibold text-foreground-subtle mb-0.5">Consignment ID</p>
          <p className="text-xs font-mono text-foreground-muted">{consignment.id}</p>
        </div>
        <div>
          <p className="text-xs font-semibold text-foreground-subtle mb-0.5">Date Created</p>
          <p className="text-xs text-foreground-muted">{formatDateTime(consignment.createdAt)}</p>
        </div>
      </div>

      <div className="bg-background rounded-lg shadow flex flex-col flex-1 min-h-0 relative">
        {refreshing && (
          <div className="absolute inset-0 bg-background/80 backdrop-blur-sm z-20 flex items-center justify-center rounded-lg">
            <div className="flex items-center gap-3 bg-background px-6 py-4 rounded-lg shadow-lg">
              <Spinner size="3" />
              <Text size="3" weight="medium" color="gray">
                Refreshing...
              </Text>
            </div>
          </div>
        )}

        <div className="p-4 flex-1 flex flex-col min-h-0">
          {workflowNodes.length > 0 ? (
            <div className="flex-1 min-h-0 flex flex-col overflow-hidden">
              <ActionListView
                className="flex-1 min-h-0"
                steps={workflowNodes}
                consignmentId={consignmentId!}
                onRefresh={handleRefresh}
                refreshing={refreshing}
                consignmentState={consignment.state}
              />
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
      </div>
    </div>
  )
}
