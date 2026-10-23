import { useState, useEffect, useCallback } from 'react'
import { useParams, useNavigate } from 'react-router-dom'
import { Button, IconButton, Spinner, Text, Tooltip } from '@radix-ui/themes'
import { ArrowLeftIcon, ArrowRightIcon, ReloadIcon } from '@radix-ui/react-icons'
import { useTranslation } from 'react-i18next'
import { getZoneView, saveTaskStepDraft, submitTaskStep } from './service'
import { HttpError } from '@/services/http'
import { getConsignment } from '@/features/consignment/service.ts'
import type { Consignment, WorkflowNode } from '@/features/consignment/types'
import { isTraderVisibleNodeType } from '@/features/consignment/workflowNodes'
import { TraderZoneLayout } from '@/features/zone/components/TraderZoneLayout'
import type { ZoneView } from '@/features/zone/types'
import { zoneWidthClass } from '@/features/zone/width'
import { TaskBreadcrumb } from './TaskBreadcrumb'

const POST_SUBMIT_REFETCH_DELAY_MS = 1500
const NEXT_TASK_MAX_ATTEMPTS = 5
const NEXT_TASK_RETRY_MS = 1000
const ACTIONABLE_NODE_STATES = new Set(['READY', 'IN_PROGRESS'])

function nextActionableTaskId(nodes: WorkflowNode[], currentTaskId: string): string | undefined {
  const actionable = nodes.filter((node) => {
    if (node.id === currentTaskId) return false
    if (!isTraderVisibleNodeType(node.workflowNodeTemplate.type)) return false
    return ACTIONABLE_NODE_STATES.has(node.state)
  })
  const successor = actionable.find((node) => node.depends_on?.includes(currentTaskId))
  return (successor ?? actionable[0])?.id
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
  // Bumped once an action's refetch has landed, and mixed into the zone keys so
  // the form remounts against what came back. A form seeds its data on mount
  // and deliberately ignores later polls, which is what stops a background
  // refresh clobbering someone mid-sentence -- but it also means a step that
  // returns to the same form (verify, save) would otherwise keep displaying the
  // values it mounted with, from before the submission was saved.
  const [formEpoch, setFormEpoch] = useState(0)
  // The trader's next task once this one is done: undefined while it's still
  // being looked up, null once the lookup found none.
  const [nextTaskId, setNextTaskId] = useState<string | null | undefined>(undefined)
  // Set once a step of this task is submitted here. The task that follows
  // starts a moment after a submit, so only then is the lookup retried.
  const [submittedHere, setSubmittedHere] = useState(false)
  // The consignment, for the breadcrumb and the task names.
  const [consignment, setConsignment] = useState<Consignment | null>(null)
  const [prevTaskId, setPrevTaskId] = useState(taskId)
  if (taskId !== prevTaskId) {
    setPrevTaskId(taskId)
    setHasSubmitted(false)
    setSubmittedHere(false)
    setNextTaskId(undefined)
  }

  const fetchTask = useCallback(async (): Promise<ZoneView | undefined> => {
    if (!taskId) return
    setRefreshing(true)
    setError(null)
    try {
      const zv = await getZoneView(taskId)
      setZoneView(zv)
      return zv
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

  // The zone view lags the URL until the new task loads.
  const taskState = zoneView?.task_id === taskId ? zoneView?.state : undefined

  // Loads the consignment, and once the task is done, finds the next task.
  // The only writer of both: a change of consignment, task or state cancels
  // the run, so a late response never overwrites a newer one.
  useEffect(() => {
    if (!consignmentId || !taskId) return
    const findNext = taskState === 'COMPLETED'
    const maxAttempts = findNext && submittedHere ? NEXT_TASK_MAX_ATTEMPTS : 1

    let cancelled = false
    let attempt = 0
    let timeout: ReturnType<typeof setTimeout> | undefined

    const load = async () => {
      attempt += 1
      let id: string | undefined
      try {
        const loaded = await getConsignment(consignmentId)
        if (cancelled) return
        if (loaded) setConsignment(loaded)
        if (!findNext) return
        id = loaded ? nextActionableTaskId(loaded.workflowNodes ?? [], taskId) : undefined
      } catch (err) {
        if (cancelled) return
        console.error('TaskDetailScreen: failed to load consignment:', err)
        if (!findNext) return
      }
      if (id) setNextTaskId(id)
      else if (attempt < maxAttempts) timeout = setTimeout(() => void load(), NEXT_TASK_RETRY_MS)
      else setNextTaskId(null)
    }

    void load()
    return () => {
      cancelled = true
      if (timeout) clearTimeout(timeout)
    }
  }, [consignmentId, taskId, taskState, submittedHere])

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
        <div className="bg-app-surface rounded-2xl shadow-md p-6 text-center">
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
        <div className="bg-app-surface rounded-2xl shadow-md p-6 text-center">
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

  const width = zoneWidthClass(zoneView)
  // Only the consignment in the URL: the screen stays mounted across task URLs.
  const current = consignment?.id === consignmentId ? consignment : null
  const nodeName = (id: string | null | undefined) =>
    current?.workflowNodes?.find((node) => node.id === id)?.workflowNodeTemplate.name
  const taskName = nodeName(taskId)
  const nextTaskName = nodeName(nextTaskId)

  return (
    <div className="min-h-full">
      <div className={`${width} mx-auto px-4 sm:px-6 lg:px-8 pt-6 flex items-center justify-between gap-4`}>
        <TaskBreadcrumb consignmentId={consignmentId} consignmentName={current?.name} taskName={taskName} />
        <Tooltip content={t('tasks.refresh')}>
          <IconButton
            type="button"
            variant="soft"
            color="gray"
            onClick={() => void fetchTask()}
            disabled={refreshing}
            aria-label={t('tasks.refresh')}
          >
            <ReloadIcon className={refreshing ? 'animate-spin' : ''} />
          </IconButton>
        </Tooltip>
      </div>
      {submitError && (
        <div className={`${width} mx-auto px-4 sm:px-6 lg:px-8 pt-4`}>
          <div className="rounded-xl bg-error-subtle px-4 py-3 shadow-sm">
            <Text size="2" weight="medium" className="text-error-strong">
              {submitError}
            </Text>
          </div>
        </div>
      )}
      <TraderZoneLayout
        task={zoneView}
        title={taskName}
        formEpoch={formEpoch}
        onSubmitForm={
          hasSubmitted
            ? undefined
            : async (command, data, options) => {
                if (!taskId || !zoneView.step_id) return
                setSubmitError(null)
                if (options?.draft) {
                  try {
                    // The step stays where it is, so the form stays mounted with
                    // what the user typed: the server now holds the same values.
                    // The form shows "Draft saved" next to its buttons.
                    await saveTaskStepDraft(taskId, zoneView.step_id, data)
                  } catch (err) {
                    if (err instanceof HttpError && err.status === 409) {
                      setSubmitError(t('tasks.error.staleStep'))
                      await fetchTask()
                      setFormEpoch((n) => n + 1)
                    } else {
                      setSubmitError(t('tasks.error.draftFailed'))
                    }
                    console.error('TaskDetailScreen: failed to save draft:', err)
                    // Reject, so the form does not say the draft was saved.
                    throw err
                  }
                  return
                }
                try {
                  await submitTaskStep(taskId, zoneView.step_id, command, data)
                  // Latch the action off during the transition window so the step
                  // can't be double-submitted while the backend advances.
                  setHasSubmitted(true)
                  setSubmittedHere(true)
                  window.scrollTo({ top: 0, behavior: 'smooth' })

                  await new Promise((resolve) => setTimeout(resolve, POST_SUBMIT_REFETCH_DELAY_MS))
                  await fetchTask()
                  // Reseed from what the refetch returned, not from what the
                  // form mounted with. Safe to discard the on-screen values
                  // here: the submission that just succeeded carried them.
                  setFormEpoch((n) => n + 1)
                } catch (err) {
                  // Use a local error here rather than the screen-level `error`, which
                  // would unmount the layout and discard the user's entered form data.
                  if (err instanceof HttpError && err.status === 409) {
                    // The task moved on since this view loaded (another tab, a callback).
                    // Show where it is now rather than retrying against the old step.
                    setSubmitError(t('tasks.error.staleStep'))
                    await fetchTask()
                    setFormEpoch((n) => n + 1)
                  } else {
                    setSubmitError(t('tasks.error.submitFailed'))
                  }
                  console.error('TaskDetailScreen: failed to submit task step:', err)
                  // Reject, so the form keeps treating its input as unsaved.
                  throw err
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
      {zoneView.state === 'COMPLETED' && (
        <div className={`${width} mx-auto px-4 sm:px-6 lg:px-8 pb-8 flex justify-end`}>
          {nextTaskId === undefined ? (
            // Held until the lookup settles, so "Back" can't be clicked just
            // before "Continue" would have appeared.
            <Button size="3" loading disabled>
              {t('tasks.continueToNext')}
            </Button>
          ) : nextTaskId ? (
            <Button size="3" onClick={() => goToNextTask(nextTaskId)}>
              {nextTaskName ? t('tasks.continueTo', { name: nextTaskName }) : t('tasks.continueToNext')}
              <ArrowRightIcon />
            </Button>
          ) : (
            <Button size="3" variant="soft" onClick={goToTasks}>
              {t('tasks.backToConsignment')}
            </Button>
          )}
        </div>
      )}
    </div>
  )
}
