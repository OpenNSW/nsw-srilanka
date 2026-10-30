import { act, renderHook, waitFor } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { POST_SUBMIT_REFETCH_DELAY_MS, useTask } from '@/features/task/hooks'
import { claimTask, getZoneView, releaseTask, saveTaskStepDraft, submitTaskStep } from '@/features/task/service'
import { getConsignment } from '@/features/consignment/service'
import type { ConsignmentDetail } from '@/features/consignment/types'
import type { ZoneView } from '@/features/zone/types'
import { HttpError } from '@/services/http'

vi.mock('@/features/task/service', () => ({
  claimTask: vi.fn(),
  getZoneView: vi.fn(),
  releaseTask: vi.fn(),
  saveTaskStepDraft: vi.fn(),
  submitTaskStep: vi.fn(),
}))
vi.mock('@/features/consignment/service', () => ({ getConsignment: vi.fn() }))

const mockGetZoneView = vi.mocked(getZoneView)
const mockGetConsignment = vi.mocked(getConsignment)
const mockClaimTask = vi.mocked(claimTask)
const mockReleaseTask = vi.mocked(releaseTask)
const mockSaveDraft = vi.mocked(saveTaskStepDraft)
const mockSubmit = vi.mocked(submitTaskStep)

function makeZoneView(state = 'IN_PROGRESS', stepId = 'step-1'): ZoneView {
  return {
    task_id: 't-1',
    task_type: 'SIMPLE_FORM',
    state,
    step_id: stepId,
    view: [],
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
  }
}

const consignment = { id: 'c-1', name: 'Tea export' } as ConsignmentDetail

async function renderLoaded() {
  const hook = renderHook(({ taskId }) => useTask(taskId, 'c-1'), { initialProps: { taskId: 't-1' } })
  await waitFor(() => expect(hook.result.current.zoneView).not.toBeNull())
  return hook
}

describe('useTask', () => {
  beforeEach(() => {
    vi.spyOn(console, 'error').mockImplementation(() => {})
    mockGetZoneView.mockResolvedValue(makeZoneView())
    mockGetConsignment.mockResolvedValue(consignment)
  })

  afterEach(() => {
    vi.useRealTimers()
    vi.resetAllMocks()
    vi.restoreAllMocks()
  })

  it('loads the task and its consignment', async () => {
    const { result } = renderHook(() => useTask('t-1', 'c-1'))
    expect(result.current.loading).toBe(true)

    await waitFor(() => expect(result.current.loading).toBe(false))
    await waitFor(() => expect(result.current.consignment).toEqual(consignment))
    expect(result.current.zoneView).toEqual(makeZoneView())
    expect(result.current.error).toBeNull()
    expect(mockGetZoneView).toHaveBeenCalledWith('t-1', expect.any(AbortSignal))
    expect(mockGetConsignment).toHaveBeenCalledWith('c-1', expect.any(AbortSignal))
  })

  it('loads no consignment for a task outside one', async () => {
    const { result } = renderHook(() => useTask('t-1', undefined))

    await waitFor(() => expect(result.current.zoneView).not.toBeNull())
    expect(result.current.consignment).toBeNull()
    expect(mockGetConsignment).not.toHaveBeenCalled()
  })

  it('reports a failed load and recovers on refetch', async () => {
    mockGetZoneView.mockRejectedValueOnce(new Error('boom'))

    const { result } = renderHook(() => useTask('t-1', 'c-1'))
    await waitFor(() => expect(result.current.error).not.toBeNull())

    act(() => result.current.refetch())
    expect(result.current.loading).toBe(true)
    await waitFor(() => expect(result.current.zoneView).not.toBeNull())
    expect(result.current.error).toBeNull()
  })

  it('refetches the task after a claim', async () => {
    mockClaimTask.mockResolvedValue(undefined)
    const { result } = await renderLoaded()

    await act(async () => {
      await result.current.changeClaim('claim')
    })

    expect(mockClaimTask).toHaveBeenCalledWith('t-1')
    expect(mockGetZoneView).toHaveBeenCalledTimes(2)
    expect(result.current.claimError).toBeNull()
    expect(result.current.claimBusy).toBe(false)
  })

  it('explains a refused claim from the refetched task', async () => {
    mockClaimTask.mockRejectedValue(new HttpError(409, 'Conflict', null))
    const { result } = await renderLoaded()

    mockGetZoneView.mockResolvedValue(makeZoneView('COMPLETED'))
    await act(async () => {
      await result.current.changeClaim('claim')
    })
    expect(result.current.claimError).toBe('completed')

    mockGetZoneView.mockResolvedValue(makeZoneView())
    await act(async () => {
      await result.current.changeClaim('claim')
    })
    expect(result.current.claimError).toBe('changed')
  })

  it('explains a release refused because another user holds the claim', async () => {
    mockReleaseTask.mockRejectedValue(new HttpError(403, 'Forbidden', null))
    const { result } = await renderLoaded()

    await act(async () => {
      await result.current.changeClaim('release')
    })

    expect(result.current.claimError).toBe('notHolder')
    expect(mockGetZoneView).toHaveBeenCalledTimes(2)
  })

  it('saves a draft without refetching or remounting the form', async () => {
    mockSaveDraft.mockResolvedValue(undefined)
    const { result } = await renderLoaded()

    await act(async () => {
      await result.current.saveDraft({ a: 1 })
    })

    expect(mockSaveDraft).toHaveBeenCalledWith('t-1', 'step-1', { a: 1 })
    expect(mockGetZoneView).toHaveBeenCalledTimes(1)
    expect(result.current.formEpoch).toBe(0)
    expect(result.current.actionError).toBeNull()
  })

  it('reloads the form when a draft targets a step the task has left', async () => {
    mockSaveDraft.mockRejectedValue(new HttpError(409, 'Conflict', null))
    const { result } = await renderLoaded()

    await act(async () => {
      await expect(result.current.saveDraft({ a: 1 })).rejects.toBeInstanceOf(HttpError)
    })

    expect(result.current.actionError).toBe('staleStep')
    expect(mockGetZoneView).toHaveBeenCalledTimes(2)
    expect(result.current.formEpoch).toBe(1)
  })

  it('reports a failed draft save', async () => {
    mockSaveDraft.mockRejectedValue(new Error('down'))
    const { result } = await renderLoaded()

    await act(async () => {
      await expect(result.current.saveDraft({ a: 1 })).rejects.toThrow('down')
    })

    expect(result.current.actionError).toBe('draftFailed')
    expect(mockGetZoneView).toHaveBeenCalledTimes(1)
  })

  it('latches a submission until the task is refetched after a delay', async () => {
    mockSubmit.mockResolvedValue(undefined)
    const { result } = await renderLoaded()
    vi.useFakeTimers()
    const scrollTo = vi.spyOn(window, 'scrollTo').mockImplementation(() => {})

    let submitted!: Promise<void>
    await act(async () => {
      submitted = result.current.submit('SUBMIT', { a: 1 })
      await vi.advanceTimersByTimeAsync(0)
    })
    expect(mockSubmit).toHaveBeenCalledWith('t-1', 'step-1', 'SUBMIT', { a: 1 })
    expect(result.current.submitted).toBe(true)
    expect(scrollTo).toHaveBeenCalled()
    expect(mockGetZoneView).toHaveBeenCalledTimes(1)

    await act(async () => {
      await vi.advanceTimersByTimeAsync(POST_SUBMIT_REFETCH_DELAY_MS)
      await submitted
    })
    expect(mockGetZoneView).toHaveBeenCalledTimes(2)
    expect(result.current.submitted).toBe(false)
    expect(result.current.formEpoch).toBe(1)
    scrollTo.mockRestore()
  })

  it('reloads the form when a submission targets a step the task has left', async () => {
    mockSubmit.mockRejectedValue(new HttpError(409, 'Conflict', null))
    const { result } = await renderLoaded()

    await act(async () => {
      await expect(result.current.submit('SUBMIT', {})).rejects.toBeInstanceOf(HttpError)
    })

    expect(result.current.actionError).toBe('staleStep')
    expect(result.current.formEpoch).toBe(1)
    expect(result.current.submitted).toBe(false)
  })

  it('reports a failed submission without refetching', async () => {
    mockSubmit.mockRejectedValue(new Error('down'))
    const { result } = await renderLoaded()

    await act(async () => {
      await expect(result.current.submit('SUBMIT', {})).rejects.toThrow('down')
    })

    expect(result.current.actionError).toBe('submitFailed')
    expect(mockGetZoneView).toHaveBeenCalledTimes(1)
    expect(result.current.formEpoch).toBe(0)
  })

  it('clears the claim error when the task changes', async () => {
    mockClaimTask.mockRejectedValue(new Error('down'))
    const { result, rerender } = await renderLoaded()

    await act(async () => {
      await result.current.changeClaim('claim')
    })
    expect(result.current.claimError).toBe('failed')

    rerender({ taskId: 't-2' })
    expect(result.current.claimError).toBeNull()
  })
})
