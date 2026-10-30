import { act, renderHook, waitFor } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { MAX_POLLS, useConsignment } from '@/features/consignment/hooks'
import { getConsignment, startConsignmentFlow } from '@/features/consignment/service'
import type { ConsignmentDetail, ConsignmentState, WorkflowNode } from '@/features/consignment/types'

vi.mock('@/features/consignment/service', () => ({ getConsignment: vi.fn(), startConsignmentFlow: vi.fn() }))

const mockGetConsignment = vi.mocked(getConsignment)
const mockStartFlow = vi.mocked(startConsignmentFlow)

function makeNode(id: string): WorkflowNode {
  return {
    id,
    createdAt: '2026-01-01T00:00:00Z',
    updatedAt: '2026-01-01T00:00:00Z',
    workflowNodeTemplate: { name: id, description: '', type: 'SIMPLE_FORM' },
    state: 'READY',
    depends_on: [],
  }
}

function makeConsignment(nodeIds: string[], state: ConsignmentState = 'IN_PROGRESS'): ConsignmentDetail {
  return {
    id: 'c-1',
    flow: 'EXPORT',
    traderId: 't-1',
    state,
    items: [],
    globalContext: {} as ConsignmentDetail['globalContext'],
    createdAt: '2026-01-01T00:00:00Z',
    updatedAt: '2026-01-01T00:00:00Z',
    workflowNodes: nodeIds.map(makeNode),
  }
}

describe('useConsignment', () => {
  afterEach(() => {
    vi.useRealTimers()
    mockGetConsignment.mockReset()
    mockStartFlow.mockReset()
  })

  it('loads the consignment without polling once its workflow exists', async () => {
    vi.useFakeTimers()
    mockGetConsignment.mockResolvedValue(makeConsignment(['n-1']))

    const { result } = renderHook(() => useConsignment('c-1'))
    expect(result.current.loading).toBe(true)

    await act(async () => {
      await vi.advanceTimersByTimeAsync(10000)
    })
    expect(result.current.loading).toBe(false)
    expect(result.current.provisioning).toBe(false)
    expect(result.current.refreshing).toBe(false)
    expect(result.current.error).toBeNull()
    expect(result.current.consignment?.workflowNodes).toHaveLength(1)
    expect(mockGetConsignment).toHaveBeenCalledTimes(1)
    expect(mockGetConsignment).toHaveBeenCalledWith('c-1', expect.any(AbortSignal))
  })

  it('reports a missing ID without fetching', () => {
    const { result } = renderHook(() => useConsignment(undefined))

    expect(result.current.error).toBe('idRequired')
    expect(result.current.loading).toBe(false)
    expect(mockGetConsignment).not.toHaveBeenCalled()
  })

  it('reports a consignment the fetcher cannot find', async () => {
    mockGetConsignment.mockResolvedValue(null)

    const { result } = renderHook(() => useConsignment('c-1'))

    await waitFor(() => expect(result.current.error).toBe('notFound'))
    expect(result.current.consignment).toBeNull()
  })

  it('reports a failed load and recovers on refetch', async () => {
    mockGetConsignment.mockRejectedValueOnce(new Error('boom')).mockResolvedValue(makeConsignment(['n-1']))

    const { result } = renderHook(() => useConsignment('c-1'))
    await waitFor(() => expect(result.current.error).toBe('loadFailed'))

    act(() => result.current.refetch())
    expect(result.current.loading).toBe(true)
    await waitFor(() => expect(result.current.consignment).not.toBeNull())
    expect(result.current.error).toBeNull()
  })

  it('polls with backoff while the workflow is provisioned', async () => {
    vi.useFakeTimers()
    mockGetConsignment
      .mockResolvedValueOnce(makeConsignment([]))
      .mockResolvedValueOnce(makeConsignment([]))
      .mockResolvedValue(makeConsignment(['n-1']))

    const { result } = renderHook(() => useConsignment('c-1'))
    await act(async () => {
      await vi.advanceTimersByTimeAsync(0)
    })
    expect(result.current.provisioning).toBe(true)
    expect(mockGetConsignment).toHaveBeenCalledTimes(1)

    await act(async () => {
      await vi.advanceTimersByTimeAsync(1000)
    })
    expect(mockGetConsignment).toHaveBeenCalledTimes(2)
    expect(result.current.provisioning).toBe(true)

    await act(async () => {
      await vi.advanceTimersByTimeAsync(1999)
    })
    expect(mockGetConsignment).toHaveBeenCalledTimes(2)

    await act(async () => {
      await vi.advanceTimersByTimeAsync(1)
    })
    expect(mockGetConsignment).toHaveBeenCalledTimes(3)
    expect(result.current.provisioning).toBe(false)
    expect(result.current.consignment?.workflowNodes).toHaveLength(1)
  })

  it('does not poll a finished consignment without a workflow', async () => {
    vi.useFakeTimers()
    mockGetConsignment.mockResolvedValue(makeConsignment([], 'FINISHED'))

    const { result } = renderHook(() => useConsignment('c-1'))
    await act(async () => {
      await vi.advanceTimersByTimeAsync(10000)
    })

    expect(result.current.provisioning).toBe(false)
    expect(mockGetConsignment).toHaveBeenCalledTimes(1)
  })

  it('gives up provisioning after MAX_POLLS polls', async () => {
    vi.useFakeTimers()
    mockGetConsignment.mockResolvedValue(makeConsignment([]))

    const { result } = renderHook(() => useConsignment('c-1'))
    await act(async () => {
      await vi.advanceTimersByTimeAsync(60000)
    })

    expect(mockGetConsignment).toHaveBeenCalledTimes(MAX_POLLS + 1)
    expect(result.current.provisioning).toBe(false)
    expect(result.current.error).toBeNull()
    expect(result.current.consignment?.workflowNodes).toHaveLength(0)
  })

  it('keeps polling through a transient failure', async () => {
    vi.useFakeTimers()
    mockGetConsignment
      .mockResolvedValueOnce(makeConsignment([]))
      .mockRejectedValueOnce(new Error('blip'))
      .mockResolvedValue(makeConsignment(['n-1']))

    const { result } = renderHook(() => useConsignment('c-1'))
    await act(async () => {
      await vi.advanceTimersByTimeAsync(3000)
    })

    expect(mockGetConsignment).toHaveBeenCalledTimes(3)
    expect(result.current.error).toBeNull()
    expect(result.current.provisioning).toBe(false)
    expect(result.current.consignment?.workflowNodes).toHaveLength(1)
  })

  it('starts a flow and refreshes until its first step appears', async () => {
    vi.useFakeTimers()
    mockStartFlow.mockResolvedValue(undefined)
    mockGetConsignment
      .mockResolvedValueOnce(makeConsignment(['n-1']))
      .mockResolvedValueOnce(makeConsignment(['n-1']))
      .mockResolvedValue(makeConsignment(['n-1', 'n-2']))

    const { result } = renderHook(() => useConsignment('c-1'))
    await act(async () => {
      await vi.advanceTimersByTimeAsync(0)
    })

    await act(async () => {
      await result.current.startFlow('fee')
    })
    expect(mockStartFlow).toHaveBeenCalledWith('c-1', 'fee')
    expect(result.current.refreshing).toBe(true)
    expect(result.current.provisioning).toBe(false)

    await act(async () => {
      await vi.advanceTimersByTimeAsync(0)
    })
    expect(mockGetConsignment).toHaveBeenCalledTimes(2)
    expect(result.current.refreshing).toBe(true)

    await act(async () => {
      await vi.advanceTimersByTimeAsync(1000)
    })
    expect(mockGetConsignment).toHaveBeenCalledTimes(3)
    expect(result.current.refreshing).toBe(false)
    expect(result.current.consignment?.workflowNodes).toHaveLength(2)

    await act(async () => {
      await vi.advanceTimersByTimeAsync(10000)
    })
    expect(mockGetConsignment).toHaveBeenCalledTimes(3)
  })

  it('rejects startFlow without reloading when the start fails', async () => {
    mockStartFlow.mockRejectedValue(new Error('forbidden'))
    mockGetConsignment.mockResolvedValue(makeConsignment(['n-1']))

    const { result } = renderHook(() => useConsignment('c-1'))
    await waitFor(() => expect(result.current.consignment).not.toBeNull())

    await act(async () => {
      await expect(result.current.startFlow('fee')).rejects.toThrow('forbidden')
    })
    expect(mockGetConsignment).toHaveBeenCalledTimes(1)
    expect(result.current.refreshing).toBe(false)
  })

  it('fetches through the given fetcher, keyed by scope', async () => {
    const adminFetcher = vi.fn().mockResolvedValue(makeConsignment(['n-1']))

    const { result, rerender } = renderHook(({ scope }) => useConsignment('c-1', { scope, fetcher: adminFetcher }), {
      initialProps: { scope: 'admin' },
    })
    await waitFor(() => expect(result.current.consignment).not.toBeNull())
    expect(adminFetcher).toHaveBeenCalledWith('c-1', expect.any(AbortSignal))
    expect(mockGetConsignment).not.toHaveBeenCalled()

    rerender({ scope: 'other' })
    await waitFor(() => expect(adminFetcher).toHaveBeenCalledTimes(2))
  })
})
