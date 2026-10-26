import { useRef, useState } from 'react'
import { useMutation } from '@/hooks/useMutation'
import { useQuery } from '@/hooks/useQuery'
import { HttpError } from '@/services/http'
import { getConsignment } from '@/features/consignment/service.ts'
import type { ConsignmentDetail } from '@/features/consignment/types'
import type { ZoneView } from '@/features/zone/types'
import { claimTask, getZoneView, releaseTask, saveTaskStepDraft, submitTaskStep } from './service'

// The next step starts asynchronously, so a single refetch is too early to call the
// submit a failure. Poll until the step moves, then warn.
export const SUBMIT_POLL_INTERVAL_MS = 1000
export const SUBMIT_POLL_WARN_MS = 25_000

export type ClaimOp = 'claim' | 'release'

// Why a claim, takeover or release was refused.
export type ClaimErrorKey = 'completed' | 'changed' | 'notHolder' | 'unavailable' | 'failed'

// Why a draft save or a submission failed.
export type ActionErrorKey = 'staleStep' | 'draftFailed' | 'submitFailed'

export interface UseTaskResult {
  zoneView: ZoneView | null
  // The consignment, for the breadcrumb and the task names, or null until it loads.
  consignment: ConsignmentDetail | null
  loading: boolean
  error: Error | null
  refreshing: boolean
  refetch: () => void
  // Bumped once an action's refetch has landed, and mixed into the zone keys so the form
  // remounts against what came back. A form seeds its data on mount and deliberately ignores
  // later polls, which is what stops a background refresh clobbering someone mid-sentence --
  // but it also means a step that returns to the same form (verify, save) would otherwise
  // keep displaying the values it mounted with, from before the submission was saved.
  formEpoch: number
  // True from a successful submission until the task has been refetched, so the step can't be
  // submitted twice while the backend advances.
  submitted: boolean
  // True when the poll ended with the task still on this step in PENDING_USER.
  stillProcessing: boolean
  actionError: ActionErrorKey | null
  claimBusy: boolean
  claimError: ClaimErrorKey | null
  changeClaim: (op: ClaimOp) => Promise<void>
  // Both reject on failure, so the form keeps treating its input as unsaved.
  saveDraft: (data: Record<string, unknown>) => Promise<void>
  submit: (command: string, data: Record<string, unknown>) => Promise<void>
}

export function useTask(taskId: string | undefined, consignmentId: string | undefined): UseTaskResult {
  const task = useQuery(taskId ? ['task', taskId] : null, (signal) => getZoneView(taskId!, signal))
  // Reloaded per task too, so the task names follow the workflow as it advances.
  const consignment = useQuery(taskId && consignmentId ? ['consignment', consignmentId, taskId] : null, (signal) =>
    getConsignment(consignmentId!, signal),
  )

  const [formEpoch, setFormEpoch] = useState(0)
  const [submitted, setSubmitted] = useState(false)
  const [stillProcessing, setStillProcessing] = useState(false)
  const [actionError, setActionError] = useState<ActionErrorKey | null>(null)
  const [claimError, setClaimError] = useState<ClaimErrorKey | null>(null)
  // A polled view, so a fetch that fails during the wait does not mark the query failed
  // and replace the form with the load error. Null until a poll or a refetch publishes one.
  const [freshView, setFreshView] = useState<ZoneView | null>(null)
  const [prevTaskId, setPrevTaskId] = useState(taskId)
  // Bumped when the task changes, and when a poll starts, so a poll for the
  // task the trader left cannot write this screen.
  const pollGeneration = useRef(0)
  if (taskId !== prevTaskId) {
    setPrevTaskId(taskId)
    // The in-flight poll reads this as soon as the task id changes, before the
    // next effect, so the bump has to happen here.
    // eslint-disable-next-line react-hooks/refs
    pollGeneration.current += 1
    setSubmitted(false)
    setStillProcessing(false)
    setActionError(null)
    setClaimError(null)
    setFreshView(null)
  }

  const view = freshView ?? task.data

  const publish = (zv: ZoneView | undefined) => {
    if (zv) setFreshView(zv)
    return zv
  }

  // Reloads the task and reseeds the form from what came back, not from what it mounted with.
  const reloadForm = async () => {
    const zv = await task.refetch()
    publish(zv)
    setFormEpoch((n) => n + 1)
    return zv
  }

  const claim = useMutation(async (op: ClaimOp) => {
    if (!taskId) return
    setClaimError(null)
    setActionError(null)
    let failure: unknown
    try {
      await (op === 'claim' ? claimTask(taskId) : releaseTask(taskId))
    } catch (err) {
      failure = err
      console.error(`useTask: failed to ${op} task:`, err)
    }
    // Refetch either way: the banner shows the claim as it is now, and every refusal means the
    // claim changed since the view loaded. The form is left as it is: a claim never changes what
    // the user may do, so what they typed is kept.
    const refreshed = publish(await task.refetch())
    if (failure !== undefined) setClaimError(claimErrorKey(failure, op, refreshed))
  })

  const saveDraft = useMutation(async (data: Record<string, unknown>) => {
    if (submitted) throw new Error('submit in progress')
    const stepId = view?.step_id
    if (!taskId || !stepId) return
    setActionError(null)
    setStillProcessing(false)
    try {
      // The step stays where it is, so the form stays mounted with what the user typed: the
      // server now holds the same values.
      await saveTaskStepDraft(taskId, stepId, data)
    } catch (err) {
      if (isConflict(err)) {
        setActionError('staleStep')
        await reloadForm()
      } else {
        setActionError('draftFailed')
      }
      console.error('useTask: failed to save draft:', err)
      throw err
    }
  })

  const submit = useMutation(async (command: string, data: Record<string, unknown>) => {
    // A second submit while the latch is on is rejected so the form does not
    // mark it saved. The handler stays mounted: the clicked button keeps its
    // label and spinner until the poll ends.
    if (submitted) throw new Error('submit in progress')
    const stepId = view?.step_id
    if (!taskId || !stepId) return
    setActionError(null)
    setStillProcessing(false)
    const generation = ++pollGeneration.current
    const pollingTaskId = taskId
    try {
      const beforeStepId = stepId
      await submitTaskStep(pollingTaskId, stepId, command, data)
      if (pollGeneration.current !== generation) return
      // Latch the action for the poll. The promise stays pending too, so the
      // clicked button stays disabled, with its spinner beside its own label.
      setSubmitted(true)
      window.scrollTo({ top: 0, behavior: 'smooth' })

      const deadline = Date.now() + SUBMIT_POLL_WARN_MS
      let latest: ZoneView | undefined
      while (Date.now() < deadline) {
        await new Promise((resolve) => setTimeout(resolve, SUBMIT_POLL_INTERVAL_MS))
        if (pollGeneration.current !== generation) return
        try {
          latest = await getZoneView(pollingTaskId)
        } catch (err) {
          console.error('useTask: failed to fetch task:', err)
          continue
        }
        if (pollGeneration.current !== generation) return
        // Hold a transient view. Publishing it would replace the form with a
        // step that has no buttons.
        if (!latest || !stepSettled(beforeStepId, latest)) continue
        publish(latest)
        break
      }
      if (pollGeneration.current !== generation) return
      if (latest && stepSettled(beforeStepId, latest)) {
        // Reseed from what the poll returned, not from what the form mounted with.
        // Safe to discard the on-screen values: the submission that just succeeded carried them.
        setFormEpoch((n) => n + 1)
      } else {
        // Same step, still pending, after the wait: the hand-off has not shown up.
        // That is not "not sent". A later trader step is also PENDING_USER, and a
        // loop stays on this step, so say it is still processing. Leave the form as it is.
        setStillProcessing(true)
      }
    } catch (err) {
      if (pollGeneration.current !== generation) throw err
      if (isConflict(err)) {
        // The task moved on since this view loaded (another tab, a callback). Show where it is
        // now rather than retrying against the old step.
        setActionError('staleStep')
        await reloadForm()
      } else {
        setActionError('submitFailed')
      }
      console.error('useTask: failed to submit task step:', err)
      throw err
    } finally {
      // Re-arm once this poll is the one still on screen. A loop (Check Status,
      // Verify) comes back to the same form and needs its button. A task change
      // already bumped the generation, so this must not clear a submit the trader
      // started on the new task.
      if (pollGeneration.current === generation) setSubmitted(false)
    }
  })

  return {
    zoneView: view ?? null,
    consignment: consignment.data ?? null,
    loading: !!taskId && task.status === 'pending',
    error: task.error,
    refreshing: task.isFetching,
    refetch: () => {
      void task.refetch().then(publish)
    },
    formEpoch,
    submitted,
    stillProcessing,
    actionError,
    claimBusy: claim.isPending,
    claimError,
    changeClaim: claim.mutate,
    saveDraft: saveDraft.mutate,
    submit: submit.mutate,
  }
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

function isConflict(err: unknown): boolean {
  return err instanceof HttpError && err.status === 409
}

// refreshed is the task as refetched after the refusal, which tells a completed task from a
// claim that changed while it was being taken over (both are 409s).
function claimErrorKey(err: unknown, op: ClaimOp, refreshed: ZoneView | undefined): ClaimErrorKey {
  if (err instanceof HttpError) {
    if (err.status === 409) return refreshed?.state === 'COMPLETED' ? 'completed' : 'changed'
    // A refused release means another user holds the claim.
    if (err.status === 403 && op === 'release') return 'notHolder'
    // The task can't be claimed, or it is gone.
    if (err.status === 404) return 'unavailable'
  }
  return 'failed'
}
