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
import { LoadError } from '@/components/common/LoadError.tsx'

const NEXT_TASK_MAX_ATTEMPTS = 5
const NEXT_TASK_RETRY_MS = 1000
// The next step starts asynchronously, so a single refetch is too early to
// call the submit a failure. Poll until the step moves, then warn.
const SUBMIT_POLL_INTERVAL_MS = 1000
const SUBMIT_POLL_WARN_MS = 25_000
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

// ADVANCING and STARTING_STEP are the gap between steps. Those views have
// no buttons, so the poll keeps going until a step the trader can use.
const TRANSIENT_STEP_STATES = new Set(['ADVANCING', 'STARTING_STEP'])

// A hand-off has landed when the task leaves this step, or leaves PENDING_USER,
// onto a view that is not still starting. Staying on the same step in
// PENDING_USER is not proof the submit failed: verify, save, and check-status
// loop there, and a trader step can hand off to another trader step that is
// still PENDING_USER on a new step id.
function stepSettled(beforeStepId: string | undefined, zv: ZoneView): boolean {
  if (TRANSIENT_STEP_STATES.has(zv.state)) return false
  return zv.step_id !== beforeStepId || zv.state !== 'PENDING_USER'
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
  const [zoneView, setZoneView] = useState<ZoneView | null>(null)
  const [loading, setLoading] = useState(true)
  const [retryCount, setRetryCount] = useState(0)
  const [refreshing, setRefreshing] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [submitError, setSubmitError] = useState<string | null>(null)
  const [submitWarning, setSubmitWarning] = useState<string | null>(null)
  // Latches a non-draft submit until its poll ends, then re-arms so a step
  // that loops back to the same form can be submitted again.
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
  // Bumped when the task changes, and when a poll starts, so a poll for the
  // task the trader left cannot write this screen.
  const pollGeneration = useRef(0)
  if (taskId !== prevTaskId) {
    setPrevTaskId(taskId)
    pollGeneration.current += 1
    setHasSubmitted(false)
    setSubmittedHere(false)
    setSubmitError(null)
    setSubmitWarning(null)
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
  }, [taskId, t, retryCount])

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
        <div className="bg-app-surface rounded-2xl shadow-md p-8 flex flex-col items-center gap-4">
          <LoadError
            message={error}
            onRetry={() => {
              setLoading(true)
              setError(null)
              setRetryCount((n) => n + 1)
            }}
          />
          <Button variant="soft" color="gray" onClick={goToTasks}>
            <ArrowLeftIcon />
            {t('tasks.goBack')}
          </Button>
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
      {submitError && (
        <div className={`${width} mx-auto px-4 sm:px-6 lg:px-8 pt-4`}>
          <div className="rounded-xl bg-error-subtle px-4 py-3 shadow-sm">
            <Text size="2" weight="medium" className="text-error-strong">
              {submitError}
            </Text>
          </div>
        </div>
      )}
      {submitWarning && (
        <div className={`${width} mx-auto px-4 sm:px-6 lg:px-8 pt-4`}>
          <div className="rounded-xl bg-warning-subtle px-4 py-3 shadow-sm">
            <Text size="2" weight="medium" className="text-warning-strong">
              {submitWarning}
            </Text>
          </div>
        </div>
      )}
      <TraderZoneLayout
        task={zoneView}
        title={title}
        formEpoch={formEpoch}
        onSubmitForm={async (command, data, options) => {
          // A second submit while the latch is on is rejected so the form
          // does not mark it saved. The handler stays mounted: the clicked
          // button keeps its label and spinner until the poll ends.
          if (hasSubmitted) throw new Error('submit in progress')
          if (!taskId || !zoneView.step_id) return
          setSubmitError(null)
          setSubmitWarning(null)
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
          const generation = ++pollGeneration.current
          const pollingTaskId = taskId
          try {
            const beforeStepId = zoneView.step_id
            await submitTaskStep(pollingTaskId, zoneView.step_id, command, data)
            if (pollGeneration.current !== generation) return
            // Latch the action for the poll. The promise stays pending too,
            // so the clicked button stays disabled, with its spinner beside
            // its own label.
            setHasSubmitted(true)
            setSubmittedHere(true)
            window.scrollTo({ top: 0, behavior: 'smooth' })

            const deadline = Date.now() + SUBMIT_POLL_WARN_MS
            let latest: ZoneView | undefined
            while (Date.now() < deadline) {
              await new Promise((resolve) => setTimeout(resolve, SUBMIT_POLL_INTERVAL_MS))
              if (pollGeneration.current !== generation) return
              try {
                latest = await getZoneView(pollingTaskId)
              } catch (err) {
                console.error('TaskDetailScreen: failed to fetch task:', err)
                continue
              }
              if (pollGeneration.current !== generation) return
              // Hold a transient view. Publishing it would replace the form
              // with a step that has no buttons.
              if (!stepSettled(beforeStepId, latest)) continue
              setZoneView(latest)
              break
            }
            if (pollGeneration.current !== generation) return
            if (latest && stepSettled(beforeStepId, latest)) {
              // Reseed from what the refetch returned, not from what the
              // form mounted with. Safe to discard the on-screen values
              // here: the submission that just succeeded carried them.
              setFormEpoch((n) => n + 1)
            } else {
              // Same step, still pending, after the wait: the hand-off has
              // not shown up. That is not "not sent". A later trader step is
              // also PENDING_USER, and a loop stays on this step, so say it
              // is still processing. Leave the form as it is.
              setSubmitWarning(t('tasks.stillProcessing'))
            }
          } catch (err) {
            if (pollGeneration.current !== generation) throw err
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
            // Re-arm once this poll is the one still on screen. A loop
            // (Check Status, Verify) comes back to the same form and needs
            // its button. A task change already bumped the generation, so
            // this must not clear a submit the trader started on the new task.
            if (pollGeneration.current === generation) setHasSubmitted(false)
          }
        }}
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
