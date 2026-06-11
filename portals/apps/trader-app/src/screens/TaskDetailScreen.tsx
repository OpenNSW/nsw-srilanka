import { useState, useEffect, useCallback, useRef } from 'react'
import { useParams, useNavigate } from 'react-router-dom'
import { Button, Spinner, Text } from '@radix-ui/themes'
import { ArrowLeftIcon } from '@radix-ui/react-icons'
import { useTranslation } from 'react-i18next'
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
  const { t } = useTranslation()
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
        setError(t('tasks.error.missingId'))
        setLoading(false)
        return
      }

      try {
        if (!silent) setLoading(true)
        if (!silent) setError(null)

        const zv = await getZoneView(taskId, api)
        setZoneView(zv)
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
          setError(t('tasks.error.fetchFailed'))
          console.error(err)
        }
      } finally {
        if (!silent) setLoading(false)
      }
    },
    [api, taskId, stopPolling, t],
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
          {t('tasks.loading')}
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
              {t('tasks.goBack')}
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
            {t('tasks.error.notFound')}
          </Text>
          <div className="mt-4">
            <Button variant="soft" onClick={goBack}>
              <ArrowLeftIcon />
              {t('tasks.goBack')}
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
          {t('tasks.back')}
        </Button>
      </div>
      <TraderZoneLayout
        task={zoneView}
        onSubmitForm={async (_command, data) => {
          if (!taskId) return
          try {
            await submitTaskStep(taskId, data, api)
            await new Promise((resolve) => setTimeout(resolve, POST_SUBMIT_REFETCH_DELAY_MS))
            await fetchTask()
          } catch (err) {
            setError(t('tasks.error.submitFailed'))
            console.error(err)
          }
        }}
      />
    </div>
  )
}
