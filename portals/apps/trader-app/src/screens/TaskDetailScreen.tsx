import { useState, useEffect, useCallback } from 'react'
import { useParams, useNavigate } from 'react-router-dom'
import { Button, Spinner, Text } from '@radix-ui/themes'
import { ArrowLeftIcon } from '@radix-ui/react-icons'
import { getTaskInfo } from '../services/task'
import { useApi } from '../services/ApiContext'
import PluginRenderer, { type RenderInfo } from '../plugins'

export function TaskDetailScreen() {
  const { taskId, consignmentId, preConsignmentId } = useParams<{
    taskId: string
    consignmentId?: string
    preConsignmentId?: string
  }>()
  const navigate = useNavigate()
  const api = useApi()
  const [renderInfo, setRenderInfo] = useState<RenderInfo | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  const parentRoute = consignmentId
    ? `/consignments/${consignmentId}`
    : preConsignmentId
      ? `/pre-consignments/${preConsignmentId}`
      : '/consignments'
  const goBack = () => {
    void navigate(parentRoute)
  }

  const fetchTask = useCallback(async () => {
    if (!taskId) {
      setError('Task ID is missing.')
      setLoading(false)
      return
    }

    try {
      setLoading(true)
      setError(null)
      const taskRenderInfo = await getTaskInfo(taskId, api)
      setRenderInfo(taskRenderInfo)
    } catch (err) {
      setError('Failed to fetch task details.')
      console.error(err)
    } finally {
      setLoading(false)
    }
  }, [api, taskId])

  useEffect(() => {
    fetchTask()
  }, [fetchTask])

  if (loading) {
    return (
      <div className="flex justify-center items-center h-full p-6">
        <Spinner size="3" />
        <Text size="3" color="gray" className="ml-3">
          Loading task...
        </Text>
      </div>
    )
  }

  if (error) {
    return (
      <div className="p-6">
        <div className="bg-white rounded-lg shadow p-6 text-center">
          <Text size="4" color="red" weight="medium">
            {error}
          </Text>
          <div className="mt-4">
            <Button variant="soft" onClick={goBack}>
              <ArrowLeftIcon />
              Go Back
            </Button>
          </div>
        </div>
      </div>
    )
  }

  if (!renderInfo) {
    return (
      <div className="p-6">
        <div className="bg-white rounded-lg shadow p-6 text-center">
          <Text size="4" color="gray" weight="medium">
            Task not found.
          </Text>
          <div className="mt-4">
            <Button variant="soft" onClick={goBack}>
              <ArrowLeftIcon />
              Go Back
            </Button>
          </div>
        </div>
      </div>
    )
  }

  return (
    <div className="p-4 sm:p-6 lg:p-8 bg-gray-50 min-h-full">
      <div className="max-w-4xl mx-auto">
        <div className="mb-6">
          <Button variant="ghost" color="gray" onClick={goBack}>
            <ArrowLeftIcon />
            Back to Tasks
          </Button>
        </div>

        <PluginRenderer response={renderInfo} onTaskUpdated={fetchTask} />
      </div>
    </div>
  )
}
