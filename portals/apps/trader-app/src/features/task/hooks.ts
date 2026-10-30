import { useState } from 'react'
import { useMutation } from '@/hooks/useMutation'
import { useQuery } from '@/hooks/useQuery'
import { HttpError } from '@/services/http'
import { getConsignment } from '@/features/consignment/service.ts'
import type { ConsignmentDetail } from '@/features/consignment/types'
import type { ZoneView } from '@/features/zone/types'
import { claimTask, getZoneView, releaseTask, saveTaskStepDraft, submitTaskStep } from './service'

export const POST_SUBMIT_REFETCH_DELAY_MS = 1500

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
  const [actionError, setActionError] = useState<ActionErrorKey | null>(null)
  const [claimError, setClaimError] = useState<ClaimErrorKey | null>(null)
  const [prevTaskId, setPrevTaskId] = useState(taskId)
  if (taskId !== prevTaskId) {
    setPrevTaskId(taskId)
    setSubmitted(false)
    setClaimError(null)
  }

  // Reloads the task and reseeds the form from what came back, not from what it mounted with.
  const reloadForm = async () => {
    await task.refetch()
    setFormEpoch((n) => n + 1)
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
    const refreshed = await task.refetch()
    if (failure !== undefined) setClaimError(claimErrorKey(failure, op, refreshed))
  })

  const saveDraft = useMutation(async (data: Record<string, unknown>) => {
    const stepId = task.data?.step_id
    if (!taskId || !stepId) return
    setActionError(null)
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
    const stepId = task.data?.step_id
    if (!taskId || !stepId) return
    setActionError(null)
    try {
      await submitTaskStep(taskId, stepId, command, data)
      setSubmitted(true)
      window.scrollTo({ top: 0, behavior: 'smooth' })
      await new Promise((resolve) => setTimeout(resolve, POST_SUBMIT_REFETCH_DELAY_MS))
      // Safe to discard the on-screen values here: the submission that just succeeded carried them.
      await reloadForm()
    } catch (err) {
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
      // Re-arm the action once the task has settled. Looping steps (e.g. the ePhyto "Check
      // Status" poll) re-enter the same state, so the button must return; terminal states expose
      // no handles and stay buttonless regardless.
      setSubmitted(false)
    }
  })

  return {
    zoneView: task.data ?? null,
    consignment: consignment.data ?? null,
    loading: !!taskId && task.status === 'pending',
    error: task.error,
    refreshing: task.isFetching,
    refetch: () => void task.refetch(),
    formEpoch,
    submitted,
    actionError,
    claimBusy: claim.isPending,
    claimError,
    changeClaim: claim.mutate,
    saveDraft: saveDraft.mutate,
    submit: submit.mutate,
  }
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
    // The deployment does not use claims, or the task is gone.
    if (err.status === 404) return 'unavailable'
  }
  return 'failed'
}
