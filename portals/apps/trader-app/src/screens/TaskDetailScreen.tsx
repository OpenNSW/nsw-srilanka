import { useState, useEffect, useCallback, useRef } from 'react'
import { useParams, useNavigate } from 'react-router-dom'
import { Button, Spinner, Text } from '@radix-ui/themes'
import { ArrowLeftIcon } from '@radix-ui/react-icons'
import { getZoneView, submitTaskStep } from '../services/task'
import { useApi } from '../services/ApiContext'
import { TraderZoneLayout } from '../zones/TraderZoneLayout'
import type { ZoneView } from '../zones/types'

const POLL_INTERVAL_MS = 3000
const POST_SUBMIT_REFETCH_DELAY_MS = 1500

export function TaskDetailScreen() {
  const { taskId } = useParams<{ taskId: string }>()
  const navigate = useNavigate()
  const goBack = () => navigate(-1)
  const api = useApi()
  const [zoneView, setZoneView] = useState<ZoneView | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const pollTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null)

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

        const zv = await getZoneView(taskId, api)
        setZoneView(zv)
        // Poll while the task is waiting on the system (no zone advertises
        // handles). The moment a zone exposes handles we're waiting on the
        // user — stop polling so we don't race their in-flight form edits.
        const awaitingUserInput = Object.values(zv.view).some((component) => (component.handles?.length ?? 0) > 0)
        if (awaitingUserInput) {
          stopPolling()
        } else {
          pollTimerRef.current = setTimeout(() => void fetchTask(true), POLL_INTERVAL_MS)
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
    void fetchTask()
    return () => stopPolling()
  }, [fetchTask, stopPolling])

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
        <div className="bg-background rounded-lg shadow p-6 text-center">
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

  if (!zoneView) {
    return (
      <div className="p-6">
        <div className="bg-background rounded-lg shadow p-6 text-center">
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
    <div className="bg-surface min-h-full">
      <div className="max-w-4xl mx-auto px-4 sm:px-6 lg:px-8 pt-6">
        <Button variant="ghost" color="gray" onClick={goBack}>
          <ArrowLeftIcon />
          Back to Tasks
        </Button>
      </div>
      <TraderZoneLayout
        task={zoneView}
        onSubmitForm={async (_command, data) => {
          if (!taskId) return
          try {
            await submitTaskStep(taskId, data, api)
            // Give Temporal a moment to advance the workflow before refetching.
            await new Promise((resolve) => setTimeout(resolve, POST_SUBMIT_REFETCH_DELAY_MS))
            await fetchTask()
          } catch (err) {
            setError('Failed to submit task. Please try again.')
            console.error(err)
          }
        }}
      />
    </div>
  )
}
