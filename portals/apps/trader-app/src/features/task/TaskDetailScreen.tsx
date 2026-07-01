import { useState, useEffect, useCallback } from 'react'
import { useParams, useNavigate } from 'react-router-dom'
import { Button, Spinner, Text } from '@radix-ui/themes'
import { ArrowLeftIcon, ReloadIcon } from '@radix-ui/react-icons'
import { useTranslation } from 'react-i18next'
import { getZoneView, submitTaskStep } from './service'
import { TraderZoneLayout } from '@/features/zone/components/TraderZoneLayout'
import type { ZoneView } from '@/features/zone/types'

const POST_SUBMIT_REFETCH_DELAY_MS = 1500

export function TaskDetailScreen() {
  const { taskId } = useParams<{ taskId: string }>()
  const navigate = useNavigate()
  const goBack = () => void navigate(-1)
  const { t } = useTranslation()
  const [zoneView, setZoneView] = useState<ZoneView | null>(null)
  const [loading, setLoading] = useState(true)
  const [refreshing, setRefreshing] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [submitError, setSubmitError] = useState<string | null>(null)
  const [hasSubmitted, setHasSubmitted] = useState(false)
  const [prevTaskId, setPrevTaskId] = useState(taskId)
  if (taskId !== prevTaskId) {
    setPrevTaskId(taskId)
    setHasSubmitted(false)
  }

  const fetchTask = useCallback(async () => {
    if (!taskId) return
    setRefreshing(true)
    setError(null)
    try {
      const zv = await getZoneView(taskId)
      setZoneView(zv)
    } catch (err) {
      setError(t('tasks.error.fetchFailed'))
      console.error('TaskDetailScreen: failed to fetch task:', err)
    } finally {
      setRefreshing(false)
    }
  }, [taskId, t])

  useEffect(() => {
    if (!taskId) return
    let cancelled = false
    void getZoneView(taskId)
      .then((zv) => {
        if (!cancelled) {
          setZoneView(zv)
          setLoading(false)
        }
      })
      .catch((err) => {
        if (!cancelled) {
          setError(t('tasks.error.fetchFailed'))
          setLoading(false)
          console.error('TaskDetailScreen: failed to fetch task:', err)
        }
      })
    return () => {
      cancelled = true
    }
  }, [taskId, t])

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
        <div className="bg-app-surface rounded-lg shadow p-6 text-center">
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
        <div className="bg-app-surface rounded-lg shadow p-6 text-center">
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
    <div className="min-h-full">
      <div className="mx-auto px-4 sm:px-6 lg:px-8 pt-6 flex items-center justify-between">
        <Button variant="ghost" color="gray" onClick={goBack}>
          <ArrowLeftIcon />
          {t('tasks.back')}
        </Button>
        <Button
          variant="soft"
          color="blue"
          size="2"
          onClick={() => void fetchTask()}
          disabled={refreshing}
          className="cursor-pointer"
        >
          <ReloadIcon className={refreshing ? 'animate-spin' : ''} />
          {t('tasks.refresh')}
        </Button>
      </div>
      {submitError && (
        <div className="mx-auto px-4 sm:px-6 lg:px-8 pt-4">
          <div className="rounded-lg border border-red-6 bg-red-2 px-4 py-3">
            <Text size="2" color="red" weight="medium">
              {submitError}
            </Text>
          </div>
        </div>
      )}
      <TraderZoneLayout
        task={zoneView}
        onSubmitForm={
          hasSubmitted
            ? undefined
            : async (command, data) => {
                if (!taskId) return
                setSubmitError(null)
                try {
                  await submitTaskStep(taskId, command, data)
                  // Latch the action off during the transition window so the step
                  // can't be double-submitted while the backend advances.
                  setHasSubmitted(true)
                  await new Promise((resolve) => setTimeout(resolve, POST_SUBMIT_REFETCH_DELAY_MS))
                  await fetchTask()
                } catch (err) {
                  // Use a local error here rather than the screen-level `error`, which
                  // would unmount the layout and discard the user's entered form data.
                  setSubmitError(t('tasks.error.submitFailed'))
                  console.error('TaskDetailScreen: failed to submit task step:', err)
                } finally {
                  // Re-arm the action once the task has settled. Looping steps (e.g.
                  // the ePhyto "Check Status" poll) re-enter the same state, so the
                  // button must return; terminal states expose no handles and stay
                  // buttonless regardless. Without this reset the button vanishes
                  // after one click until a hard refresh.
                  setHasSubmitted(false)
                }
              }
        }
      />
    </div>
  )
}
