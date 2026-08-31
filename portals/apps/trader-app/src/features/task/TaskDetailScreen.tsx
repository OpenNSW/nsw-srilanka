import { useState, useEffect, useCallback } from 'react'
import { useParams, useNavigate } from 'react-router-dom'
import { Button, Spinner, Text } from '@radix-ui/themes'
import { ArrowLeftIcon, ArrowRightIcon, ReloadIcon } from '@radix-ui/react-icons'
import { useTranslation } from 'react-i18next'
import { getZoneView, submitTaskStep } from './service'
import { getConsignment } from '@/features/consignment/service.ts'
import type { WorkflowNode } from '@/features/consignment/types'
import { TraderZoneLayout } from '@/features/zone/components/TraderZoneLayout'
import type { ZoneView } from '@/features/zone/types'

const POST_SUBMIT_REFETCH_DELAY_MS = 1500
const SUBMIT_SUCCESS_DISMISS_MS = 5000
const HIDDEN_NODE_TYPES = new Set(['START', 'END', 'GATEWAY', 'END_NODE', 'SYSTEM', 'SPLIT_TASK'])

function nextActionableTaskId(nodes: WorkflowNode[], currentTaskId: string): string | undefined {
  const currentIndex = nodes.findIndex((node) => node.id === currentTaskId)
  const following = currentIndex >= 0 ? nodes.slice(currentIndex + 1) : nodes
  return following.find((node) => {
    const type = node.workflowNodeTemplate.type?.toUpperCase()
    return !HIDDEN_NODE_TYPES.has(type ?? '') && node.state !== 'COMPLETED' && node.state !== 'FAILED'
  })?.id
}

export function TaskDetailScreen() {
  const { taskId, consignmentId } = useParams<{ taskId: string; consignmentId: string }>()
  const navigate = useNavigate()
  const goToTasks = () => {
    if (consignmentId) {
      void navigate(`/consignments/${consignmentId}`, { replace: true })
      return
    }
    void navigate(-1)
  }
  const goToNextTask = (nextId: string) => {
    if (!consignmentId) return
    void navigate(`/consignments/${consignmentId}/tasks/${nextId}`)
  }
  const { t } = useTranslation()
  const [zoneView, setZoneView] = useState<ZoneView | null>(null)
  const [loading, setLoading] = useState(true)
  const [refreshing, setRefreshing] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [submitError, setSubmitError] = useState<string | null>(null)
  const [hasSubmitted, setHasSubmitted] = useState(false)
  const [showSubmitSuccess, setShowSubmitSuccess] = useState(false)
  const [nextTaskId, setNextTaskId] = useState<string | null>(null)
  const [prevTaskId, setPrevTaskId] = useState(taskId)
  if (taskId !== prevTaskId) {
    setPrevTaskId(taskId)
    setHasSubmitted(false)
    setShowSubmitSuccess(false)
    setNextTaskId(null)
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

  useEffect(() => {
    if (!consignmentId || !taskId || zoneView?.state !== 'COMPLETED') return

    let cancelled = false
    let attempt = 0
    let timeout: ReturnType<typeof setTimeout> | undefined

    const loadNextTask = async () => {
      try {
        const consignment = await getConsignment(consignmentId)
        if (cancelled || !consignment) return
        const id = nextActionableTaskId(consignment.workflowNodes ?? [], taskId)
        setNextTaskId(id ?? null)
        if (!id && attempt < 4) {
          attempt += 1
          timeout = setTimeout(() => void loadNextTask(), 1000)
        }
      } catch (err) {
        console.error('TaskDetailScreen: failed to resolve next task:', err)
      }
    }

    void loadNextTask()
    return () => {
      cancelled = true
      if (timeout) clearTimeout(timeout)
    }
  }, [consignmentId, taskId, zoneView?.state])

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
            <Button variant="soft" onClick={goToTasks}>
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
            <Button variant="soft" onClick={goToTasks}>
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
        <div className="flex items-center gap-6">
          <Button variant="ghost" color="gray" onClick={goToTasks} className="cursor-pointer">
            <ArrowLeftIcon />
            {t('tasks.back')}
          </Button>
          {zoneView.state === 'COMPLETED' && nextTaskId && (
            <Button variant="ghost" color="gray" onClick={() => goToNextTask(nextTaskId)} className="cursor-pointer">
              {t('tasks.nextTask')}
              <ArrowRightIcon />
            </Button>
          )}
        </div>
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
      {showSubmitSuccess && (
        <div className="mx-auto px-4 sm:px-6 lg:px-8 pt-4">
          <div className="rounded-lg border border-success bg-success-subtle px-4 py-3">
            <Text size="2" weight="medium" className="text-success-strong">
              {t('tasks.submitSuccess')}
            </Text>
          </div>
        </div>
      )}
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
                  window.scrollTo({ top: 0, behavior: 'smooth' })
                  setShowSubmitSuccess(true)
                  setTimeout(() => setShowSubmitSuccess(false), SUBMIT_SUCCESS_DISMISS_MS)

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
