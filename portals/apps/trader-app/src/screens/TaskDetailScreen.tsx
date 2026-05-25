import { useState, useEffect, useCallback, useRef } from 'react'
import { useParams, useNavigate } from 'react-router-dom'
import { Button, Spinner, Text } from '@radix-ui/themes'
import { ArrowLeftIcon } from '@radix-ui/react-icons'
import { getTaskInfo } from '../services/task'
import { useApi } from '../services/ApiContext'
import PluginRenderer, { type RenderInfo } from '../plugins'
import { getBooleanEnv } from '../runtimeConfig'
import { TraderZoneLayout } from '../zones/TraderZoneLayout'
import { SAMPLE_TASK } from '../zones/fixtures'

const POLL_INTERVAL_MS = 3000
const PAYMENT_TERMINAL_STATES = ['COMPLETED', 'FAILED']
const WAIT_FOR_EVENT_TERMINAL_STATES = ['COMPLETED', 'RECEIVED_CALLBACK', 'NOTIFY_FAILED', 'SUBMISSION_FAILED']

export function TaskDetailScreen() {
  const { taskId } = useParams<{ taskId: string }>()
  const navigate = useNavigate()
  const goBack = () => navigate(-1)
  const api = useApi()
  const [renderInfo, setRenderInfo] = useState<RenderInfo | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const pollTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null)
  const useZoneRenderer = getBooleanEnv('VITE_USE_ZONE_RENDERER', false)

  const stopPolling = useCallback(() => {
    if (pollTimerRef.current) {
      clearTimeout(pollTimerRef.current)
      pollTimerRef.current = null
    }
  }, [])

  const fetchTask = useCallback(
    async (silent = false) => {
      stopPolling()
      if (!taskId) {
        setError('Task ID is missing.')
        setLoading(false)
        return
      }

      try {
        if (!silent) setLoading(true)
        if (!silent) setError(null)
        const taskRenderInfo = await getTaskInfo(taskId, api)
        setRenderInfo(taskRenderInfo)

        // Poll for tasks that are still in progress
        const { type, pluginState } = taskRenderInfo
        const shouldPoll =
          (type === 'PAYMENT' && !PAYMENT_TERMINAL_STATES.includes(pluginState)) ||
          (type === 'WAIT_FOR_EVENT' && !WAIT_FOR_EVENT_TERMINAL_STATES.includes(pluginState))
        if (shouldPoll) {
          pollTimerRef.current = setTimeout(() => void fetchTask(true), POLL_INTERVAL_MS)
        } else {
          stopPolling()
        }
      } catch (err) {
        if (silent) {
          console.error('Background poll failed:', err)
          pollTimerRef.current = setTimeout(() => void fetchTask(true), POLL_INTERVAL_MS)
        } else {
          setError('Failed to fetch task details.')
          console.error(err)
        }
      } finally {
        if (!silent) setLoading(false)
      }
    },
    [api, taskId, stopPolling],
  )

  useEffect(() => {
    if (useZoneRenderer) return
    void fetchTask()
    return () => stopPolling()
  }, [fetchTask, stopPolling, useZoneRenderer])

  if (useZoneRenderer) {
    // Flag-gated preview: backend doesn't emit the ZoneView shape yet, so we
    // render a fixture inside the real task route. The taskId from the URL is
    // intentionally ignored.
    // TODO(zones): swap fixture for fetch once /api/v1/tasks/{id} returns ZoneView,
    // then delete this branch.
    return (
      <div className="bg-gray-50 min-h-full">
        <div className="max-w-4xl mx-auto px-4 sm:px-6 lg:px-8 pt-6">
          <Button
            variant="ghost"
            color="gray"
            onClick={() => {
              void goBack()
            }}
          >
            <ArrowLeftIcon />
            Back to Tasks
          </Button>
        </div>
        <TraderZoneLayout task={SAMPLE_TASK} />
      </div>
    )
  }

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
