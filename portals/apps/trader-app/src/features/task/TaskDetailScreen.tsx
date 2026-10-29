import { useState, useEffect, useCallback } from 'react'
import { useParams, useNavigate } from 'react-router-dom'
import { Button, IconButton, Spinner, Text, Tooltip } from '@radix-ui/themes'
import { ArrowLeftIcon, ReloadIcon } from '@radix-ui/react-icons'
import { useTranslation } from 'react-i18next'
import type { TFunction } from 'i18next'
import { claimTask, getZoneView, releaseTask, saveTaskStepDraft, submitTaskStep } from './service'
import { ClaimBanner } from './ClaimBanner'
import { HttpError } from '@/services/http'
import { getConsignment } from '@/features/consignment/service.ts'
import type { Consignment } from '@/features/consignment/types'
import { TraderZoneLayout } from '@/features/zone/components/TraderZoneLayout'
import type { ZoneView } from '@/features/zone/types'
import { zoneWidthClass } from '@/features/zone/width'
import { Breadcrumb, type BreadcrumbItem } from '@/components/Breadcrumb'
import { humanizeStatus } from '@/utils/formatStatus'
import { isAgencyMode } from '@/features/case/mode'
import { ROUTES } from '@/constants'
import { LoadError } from '@/components/common/LoadError.tsx'

const POST_SUBMIT_REFETCH_DELAY_MS = 1500

type ClaimOp = 'claim' | 'release'

// claimErrorMessage explains a refused claim, takeover or release. refreshed is the
// task as refetched after the refusal, which tells a completed task from a claim that
// changed while it was being taken over (both are 409s).
function claimErrorMessage(err: unknown, op: ClaimOp, refreshed: ZoneView | undefined, t: TFunction): string {
  if (err instanceof HttpError) {
    if (err.status === 409) {
      return refreshed?.state === 'COMPLETED' ? t('tasks.claim.error.completed') : t('tasks.claim.error.changed')
    }
    // A refused release means another user holds the claim.
    if (err.status === 403 && op === 'release') return t('tasks.claim.error.notHolder')
    // The deployment does not use claims, or the task is gone.
    if (err.status === 404) return t('tasks.claim.error.unavailable')
  }
  return t('tasks.claim.error.failed')
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
  const { t } = useTranslation()
  const [zoneView, setZoneView] = useState<ZoneView | null>(null)
  const [loading, setLoading] = useState(true)
  const [retryCount, setRetryCount] = useState(0)
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
  // The consignment, for the breadcrumb and the task names.
  const [consignment, setConsignment] = useState<Consignment | null>(null)
  const [claimBusy, setClaimBusy] = useState(false)
  const [claimError, setClaimError] = useState<string | null>(null)
  const [prevTaskId, setPrevTaskId] = useState(taskId)
  if (taskId !== prevTaskId) {
    setPrevTaskId(taskId)
    setHasSubmitted(false)
    setClaimError(null)
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

  const changeClaim = async (op: ClaimOp) => {
    if (!taskId) return
    setClaimBusy(true)
    setClaimError(null)
    setSubmitError(null)
    let failure: unknown
    try {
      await (op === 'claim' ? claimTask(taskId) : releaseTask(taskId))
    } catch (err) {
      failure = err
      console.error(`TaskDetailScreen: failed to ${op} task:`, err)
    }
    // Refetch either way: the banner shows the claim as it is now, and every refusal
    // means the claim changed since the view loaded. The form is left as it is: a
    // claim never changes what the user may do, so what they typed is kept.
    const zv = await fetchTask()
    if (failure !== undefined) setClaimError(claimErrorMessage(failure, op, zv, t))
    setClaimBusy(false)
  }

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

  // Loads the consignment. A change of consignment or task cancels the run,
  // so a late response never overwrites a newer one.
  useEffect(() => {
    if (!consignmentId || !taskId) return
    let cancelled = false
    void getConsignment(consignmentId)
      .then((loaded) => {
        if (!cancelled && loaded) setConsignment(loaded)
      })
      .catch((err) => {
        if (!cancelled) console.error('TaskDetailScreen: failed to load consignment:', err)
      })
    return () => {
      cancelled = true
    }
  }, [consignmentId, taskId])

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
  // The task's configured name, or its type until the consignment has loaded.
  const title =
    current?.workflowNodes?.find((node) => node.id === taskId)?.workflowNodeTemplate.name ||
    humanizeStatus(zoneView.task_type)
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
      {zoneView.claim && (
        <div className={`${width} mx-auto px-4 sm:px-6 lg:px-8 pt-4`}>
          <ClaimBanner
            claim={zoneView.claim}
            completed={zoneView.state === 'COMPLETED'}
            busy={claimBusy}
            error={claimError}
            onClaim={() => void changeClaim('claim')}
            onRelease={() => void changeClaim('release')}
          />
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
          <Button size="3" variant="soft" onClick={goToTasks}>
            {t(isAgencyMode ? 'tasks.backToCase' : 'tasks.backToConsignment')}
          </Button>
        </div>
      )}
    </div>
  )
}
