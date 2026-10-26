import { useState, useEffect, useCallback, useRef } from 'react'
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
import { Breadcrumb, type BreadcrumbItem } from '@/components/Breadcrumb'
import { humanizeStatus } from '@/utils/formatStatus'
import { isAgencyMode } from '@/features/case/mode'
import { ROUTES } from '@/constants'
import { useActionError } from '@/components/common/actionError'
import { LoadError } from '@/components/common/LoadError.tsx'

const POST_SUBMIT_REFETCH_DELAY_MS = 1500
const NEXT_TASK_MAX_ATTEMPTS = 5
const NEXT_TASK_RETRY_MS = 1000
const ACTIONABLE_NODE_STATES = new Set(['READY', 'IN_PROGRESS'])

function isStaleTaskStep(err: unknown): boolean {
  return err instanceof HttpError && err.status === 409
}

// Quiet while the next task is still being looked up. The last failure is the one the trader sees.
function consignmentLoadWillRetry(findNext: boolean, attempt: number, maxAttempts: number): boolean {
  return findNext && attempt < maxAttempts
}

// A consignment already on screen stays up; the failure is a toast. A blank screen gets Try Again.
function consignmentAlreadyShown(shownId: string | undefined, requestedId: string): boolean {
  return shownId === requestedId
}

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
      void navigate(ROUTES.CONSIGNMENT_DETAIL(consignmentId), { replace: true })
      return
    }
    void navigate(-1)
  }
  const goToNextTask = (nextId: string) => {
    if (!consignmentId) return
    void navigate(ROUTES.TASK_FORM(consignmentId, nextId))
  }
  const { t } = useTranslation()
  const { showActionError, clearActionError } = useActionError()
  const [zoneView, setZoneView] = useState<ZoneView | null>(null)
  const [loading, setLoading] = useState(true)
  const [retryCount, setRetryCount] = useState(0)
  const [refreshing, setRefreshing] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [consignmentLoadFailed, setConsignmentLoadFailed] = useState(false)
  const [consignmentRetry, setConsignmentRetry] = useState(0)
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
  // The id on screen, read from the consignment effect without subscribing it
  // to every consignment update.
  const shownConsignmentIdRef = useRef<string | undefined>(undefined)
  // The toast raised for a failed reload of a consignment already on screen.
  const consignmentFailureToast = useRef<number | null>(null)
  // True once this task's form is on screen, so a later reload failure can toast
  // instead of replacing the form.
  const taskOnScreenRef = useRef(false)
  const [prevTaskId, setPrevTaskId] = useState(taskId)
  const [prevConsignmentId, setPrevConsignmentId] = useState(consignmentId)
  if (taskId !== prevTaskId) {
    setPrevTaskId(taskId)
    setHasSubmitted(false)
    setSubmittedHere(false)
    setNextTaskId(undefined)
  }
  if (consignmentId !== prevConsignmentId) {
    setPrevConsignmentId(consignmentId)
    setConsignmentLoadFailed(false)
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
      console.error('TaskDetailScreen: failed to fetch task:', err)
      if (taskOnScreenRef.current) {
        showActionError(t('tasks.error.fetchFailed'))
      } else {
        setError(t('tasks.error.fetchFailed'))
      }
    } finally {
      setRefreshing(false)
    }
  }, [taskId, t, showActionError])

  // A 409 means the step moved on: show that, and replace the form with the
  // task's current state. Anything else keeps the form and says the action failed.
  const reportTaskActionError = useCallback(
    async (err: unknown, fallback: 'tasks.error.draftFailed' | 'tasks.error.submitFailed') => {
      if (!isStaleTaskStep(err)) {
        showActionError(t(fallback))
        return
      }
      showActionError(t('tasks.error.staleStep'))
      await fetchTask()
      setFormEpoch((n) => n + 1)
    },
    [fetchTask, showActionError, t],
  )

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
  }, [taskId, t, retryCount])

  // The zone view lags the URL until the new task loads.
  const taskState = zoneView?.task_id === taskId ? zoneView?.state : undefined

  useEffect(() => {
    shownConsignmentIdRef.current = consignment?.id === consignmentId ? consignmentId : undefined
    taskOnScreenRef.current = zoneView?.task_id === taskId
  }, [consignment, consignmentId, zoneView, taskId])

  useEffect(() => {
    consignmentFailureToast.current = null
  }, [consignmentId])

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
        setConsignmentLoadFailed(false)
        if (consignmentFailureToast.current !== null) {
          clearActionError(consignmentFailureToast.current)
          consignmentFailureToast.current = null
        }
        if (loaded) setConsignment(loaded)
        if (!findNext) return
        id = loaded ? nextActionableTaskId(loaded.workflowNodes ?? [], taskId) : undefined
      } catch (err) {
        if (cancelled) return
        console.error('TaskDetailScreen: failed to load consignment:', err)
        if (!consignmentLoadWillRetry(findNext, attempt, maxAttempts)) {
          if (consignmentAlreadyShown(shownConsignmentIdRef.current, consignmentId)) {
            consignmentFailureToast.current = showActionError(t('consignments.detail.error.loadFailed'))
          } else {
            setConsignmentLoadFailed(true)
          }
        }
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
  }, [consignmentId, taskId, taskState, submittedHere, consignmentRetry, clearActionError, showActionError, t])

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
        <div className="bg-app-surface rounded-2xl shadow-md p-8">
          <LoadError
            message={error}
            onRetry={() => {
              setLoading(true)
              setError(null)
              setRetryCount((n) => n + 1)
            }}
            secondary={
              <Button type="button" variant="soft" color="gray" onClick={goToTasks}>
                <ArrowLeftIcon />
                {t('tasks.goBack')}
              </Button>
            }
          />
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
  // The task's configured name, or its type until the consignment has loaded.
  const title = nodeName(taskId) || humanizeStatus(zoneView.task_type)
  const nextTaskName = nodeName(nextTaskId)
  const breadcrumb: BreadcrumbItem[] = [
    { label: t(isAgencyMode ? 'nav.cases' : 'nav.consignments'), to: ROUTES.CONSIGNMENTS },
  ]
  if (consignmentId) {
    breadcrumb.push({
      label: current?.name || t(isAgencyMode ? 'cases.detail.title' : 'consignments.detail.title'),
      to: ROUTES.CONSIGNMENT_DETAIL(consignmentId),
    })
  }
  breadcrumb.push({ label: title })

  return (
    <div className="min-h-full">
      <div className={`${width} mx-auto px-4 sm:px-6 lg:px-8 pt-6 flex items-center justify-between gap-4`}>
        <Breadcrumb items={breadcrumb} />
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
      {consignmentLoadFailed && consignment?.id !== consignmentId && (
        <div className={`${width} mx-auto px-4 sm:px-6 lg:px-8 pt-6`}>
          <div className="bg-app-surface rounded-2xl shadow-md p-8">
            <LoadError
              message={t('consignments.detail.error.loadFailed')}
              description={t('consignments.detail.error.loadFailedDescription')}
              onRetry={() => setConsignmentRetry((n) => n + 1)}
            />
          </div>
        </div>
      )}
      <TraderZoneLayout
        task={zoneView}
        title={title}
        formEpoch={formEpoch}
        onSubmitForm={
          hasSubmitted
            ? undefined
            : async (command, data, options) => {
                if (!taskId || !zoneView.step_id) return
                clearActionError()
                if (options?.draft) {
                  try {
                    // The step stays where it is, so the form stays mounted with
                    // what the user typed: the server now holds the same values.
                    // The form shows "Draft saved" next to its buttons.
                    await saveTaskStepDraft(taskId, zoneView.step_id, data)
                  } catch (err) {
                    console.error('TaskDetailScreen: failed to save draft:', err)
                    await reportTaskActionError(err, 'tasks.error.draftFailed')
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
                  // A toast, not the screen-level `error`, which would unmount the
                  // layout and discard the user's entered form data.
                  console.error('TaskDetailScreen: failed to submit task step:', err)
                  await reportTaskActionError(err, 'tasks.error.submitFailed')
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
              {t(isAgencyMode ? 'tasks.backToCase' : 'tasks.backToConsignment')}
            </Button>
          )}
        </div>
      )}
    </div>
  )
}
