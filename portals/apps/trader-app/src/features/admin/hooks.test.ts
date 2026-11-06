import { act, renderHook, waitFor } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { useAdminConsignment, useEngineStatus, useForcePark, useResolveAdminIntervention } from '@/features/admin/hooks'
import {
  forceParkNode,
  getCaseForAdmin,
  getConsignmentForAdmin,
  getTaskWorkflowEngineStatus,
  getWorkflowEngineStatus,
  resolveAdminIntervention,
} from '@/features/admin/service'
import type { AdminConsignmentDetail, EngineStatus } from '@/features/admin/types'

vi.mock('@/features/admin/service', () => ({
  getConsignmentForAdmin: vi.fn(),
  getCaseForAdmin: vi.fn(),
  getWorkflowEngineStatus: vi.fn(),
  getTaskWorkflowEngineStatus: vi.fn(),
  forceParkNode: vi.fn(),
  resolveAdminIntervention: vi.fn(),
}))

const mockGetConsignment = vi.mocked(getConsignmentForAdmin)
const mockGetCase = vi.mocked(getCaseForAdmin)
const mockGetWorkflowStatus = vi.mocked(getWorkflowEngineStatus)
const mockGetTaskStatus = vi.mocked(getTaskWorkflowEngineStatus)
const mockForcePark = vi.mocked(forceParkNode)
const mockResolve = vi.mocked(resolveAdminIntervention)

function makeConsignment(id: string): AdminConsignmentDetail {
  return {
    id,
    name: `Shipment ${id}`,
    flow: 'EXPORT',
    traderId: 't-1',
    state: 'IN_PROGRESS',
    items: [],
    globalContext: {} as AdminConsignmentDetail['globalContext'],
    createdAt: '2026-01-01T00:00:00Z',
    updatedAt: '2026-01-01T00:00:00Z',
    workflowNodes: [],
    flows: [],
  }
}

function makeStatus(workflowId: string): EngineStatus {
  return { workflow_id: workflowId, status: 'RUNNING', nodes: [], audit_trail: [] }
}

describe('useAdminConsignment', () => {
  afterEach(() => {
    vi.resetAllMocks()
  })

  it('loads the consignment through the admin endpoint', async () => {
    mockGetConsignment.mockResolvedValue(makeConsignment('c-1'))

    const { result } = renderHook(() => useAdminConsignment('c-1'))
    expect(result.current.loading).toBe(true)

    await waitFor(() => expect(result.current.loading).toBe(false))
    expect(result.current.consignment?.id).toBe('c-1')
    expect(result.current.error).toBeNull()
    expect(mockGetConsignment).toHaveBeenCalledWith('c-1', expect.any(AbortSignal))
    expect(mockGetCase).not.toHaveBeenCalled()
  })

  it('reports notFound when the consignment does not exist', async () => {
    mockGetConsignment.mockResolvedValue(null)

    const { result } = renderHook(() => useAdminConsignment('missing'))

    await waitFor(() => expect(result.current.loading).toBe(false))
    expect(result.current.error).toBe('notFound')
    expect(result.current.consignment).toBeNull()
  })

  it('reports loadFailed when the request fails, and loads again on refetch', async () => {
    mockGetConsignment.mockRejectedValueOnce(new Error('network down'))
    mockGetConsignment.mockResolvedValueOnce(makeConsignment('c-1'))

    const { result } = renderHook(() => useAdminConsignment('c-1'))
    await waitFor(() => expect(result.current.error).toBe('loadFailed'))

    act(() => result.current.refetch())

    await waitFor(() => expect(result.current.consignment?.id).toBe('c-1'))
    expect(result.current.error).toBeNull()
    expect(mockGetConsignment).toHaveBeenCalledTimes(2)
  })

  it("drops the previous consignment's data while the next one loads", async () => {
    mockGetConsignment.mockResolvedValueOnce(makeConsignment('c-1'))
    mockGetConsignment.mockReturnValueOnce(new Promise(() => undefined))

    const { result, rerender } = renderHook(({ id }) => useAdminConsignment(id), { initialProps: { id: 'c-1' } })
    await waitFor(() => expect(result.current.consignment?.id).toBe('c-1'))

    rerender({ id: 'c-2' })

    expect(result.current.loading).toBe(true)
    expect(result.current.consignment).toBeNull()
    expect(mockGetConsignment).toHaveBeenLastCalledWith('c-2', expect.any(AbortSignal))
  })

  it('loads nothing without a consignment ID', () => {
    renderHook(() => useAdminConsignment(undefined))
    expect(mockGetConsignment).not.toHaveBeenCalled()
  })
})

describe('useEngineStatus', () => {
  afterEach(() => {
    vi.resetAllMocks()
  })

  it('reads a flow or child branch through the workflows route', async () => {
    mockGetWorkflowStatus.mockResolvedValue(makeStatus('wf-1'))

    const { result } = renderHook(() => useEngineStatus('wf-1', 'workflow'))

    await waitFor(() => expect(result.current.status?.workflow_id).toBe('wf-1'))
    expect(result.current.loading).toBe(false)
    expect(result.current.error).toBeNull()
    expect(mockGetWorkflowStatus).toHaveBeenCalledWith('wf-1', expect.any(AbortSignal))
    expect(mockGetTaskStatus).not.toHaveBeenCalled()
  })

  it('reads a task workflow through the task route', async () => {
    mockGetTaskStatus.mockResolvedValue(makeStatus('tw-1'))

    const { result } = renderHook(() => useEngineStatus('tw-1', 'task'))

    await waitFor(() => expect(result.current.status?.workflow_id).toBe('tw-1'))
    expect(mockGetTaskStatus).toHaveBeenCalledWith('tw-1', expect.any(AbortSignal))
    expect(mockGetWorkflowStatus).not.toHaveBeenCalled()
  })

  it('tells a missing execution apart from a failed load', async () => {
    mockGetWorkflowStatus.mockResolvedValueOnce(null)
    const notFound = renderHook(() => useEngineStatus('wf-gone', 'workflow'))
    await waitFor(() => expect(notFound.result.current.error).toBe('notFound'))

    mockGetWorkflowStatus.mockRejectedValueOnce(new Error('500'))
    const failed = renderHook(() => useEngineStatus('wf-broken', 'workflow'))
    await waitFor(() => expect(failed.result.current.error).toBe('loadFailed'))
  })

  it('keeps the status on screen while a refresh is in flight', async () => {
    mockGetWorkflowStatus.mockResolvedValueOnce(makeStatus('wf-1'))
    mockGetWorkflowStatus.mockReturnValueOnce(new Promise(() => undefined))

    const { result } = renderHook(() => useEngineStatus('wf-1', 'workflow'))
    await waitFor(() => expect(result.current.status).not.toBeNull())

    act(() => result.current.refetch())

    expect(result.current.isFetching).toBe(true)
    expect(result.current.loading).toBe(false)
    expect(result.current.status?.workflow_id).toBe('wf-1')
  })

  it('loads nothing while disabled or without a workflow ID', () => {
    renderHook(() => useEngineStatus('wf-1', 'workflow', { enabled: false }))
    renderHook(() => useEngineStatus('', 'task'))

    expect(mockGetWorkflowStatus).not.toHaveBeenCalled()
    expect(mockGetTaskStatus).not.toHaveBeenCalled()
  })

  it('loads again from scratch when the attempt changes', async () => {
    mockGetWorkflowStatus.mockRejectedValueOnce(new Error('500'))
    mockGetWorkflowStatus.mockResolvedValueOnce(makeStatus('wf-1'))

    const { result, rerender } = renderHook(({ attempt }) => useEngineStatus('wf-1', 'workflow', { attempt }), {
      initialProps: { attempt: 1 },
    })
    await waitFor(() => expect(result.current.error).toBe('loadFailed'))

    rerender({ attempt: 2 })

    expect(result.current.loading).toBe(true)
    expect(result.current.error).toBeNull()
    await waitFor(() => expect(result.current.status?.workflow_id).toBe('wf-1'))
  })
})

describe('admin actions', () => {
  afterEach(() => {
    vi.resetAllMocks()
  })

  it('force-parks a node and reports a failure', async () => {
    mockForcePark.mockResolvedValueOnce(undefined)
    mockForcePark.mockRejectedValueOnce(new Error('node is not running'))

    const { result } = renderHook(() => useForcePark())

    await act(() => result.current.mutate('wf-1', 'step-1', 'stuck callback', 'task'))
    expect(mockForcePark).toHaveBeenCalledWith('wf-1', 'step-1', 'stuck callback', 'task')
    expect(result.current.error).toBeNull()

    await act(async () => {
      await expect(result.current.mutate('wf-1', 'step-1', 'again', 'task')).rejects.toThrow('node is not running')
    })
    expect(result.current.error?.message).toBe('node is not running')
  })

  it('resolves a node through the admin endpoint', async () => {
    mockResolve.mockResolvedValue(undefined)

    const { result } = renderHook(() => useResolveAdminIntervention())
    const request = { action: 'RETRY' as const, reason: 'fixed the mapping' }

    await act(() => result.current.mutate('wf-1', 'step-1', request, 'workflow'))
    expect(mockResolve).toHaveBeenCalledWith('wf-1', 'step-1', request, 'workflow')
    expect(result.current.isPending).toBe(false)
  })
})
