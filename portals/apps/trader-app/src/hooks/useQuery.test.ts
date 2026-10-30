import { act, renderHook, waitFor } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { useQuery } from '@/hooks/useQuery'

interface Deferred<T> {
  promise: Promise<T>
  resolve: (value: T) => void
  reject: (reason?: unknown) => void
}

function createDeferred<T>(): Deferred<T> {
  let resolve!: (value: T) => void
  let reject!: (reason?: unknown) => void
  const promise = new Promise<T>((res, rej) => {
    resolve = res
    reject = rej
  })
  return { promise, resolve, reject }
}

describe('useQuery', () => {
  afterEach(() => {
    vi.useRealTimers()
  })

  it('fetches data on mount and resolves to success', async () => {
    const deferred = createDeferred<string>()
    const fetcher = vi.fn<(_signal: AbortSignal) => Promise<string>>(() => deferred.promise)

    const { result } = renderHook(() => useQuery('item-1', fetcher))

    expect(result.current.status).toBe('pending')
    expect(result.current.data).toBeUndefined()
    expect(result.current.error).toBeNull()
    expect(result.current.isFetching).toBe(true)
    expect(result.current.polling).toBe(false)
    expect(fetcher).toHaveBeenCalledTimes(1)

    await act(async () => {
      deferred.resolve('hello')
      await deferred.promise
    })

    expect(result.current.status).toBe('success')
    expect(result.current.data).toBe('hello')
    expect(result.current.error).toBeNull()
    expect(result.current.isFetching).toBe(false)
    expect(result.current.polling).toBe(false)
  })

  it('does not fetch when key is null or enabled is false, and fetches once enabled', async () => {
    const fetcher = vi.fn(() => Promise.resolve('loaded'))

    const { result, rerender } = renderHook(
      ({ key, enabled }: { key: string | null; enabled: boolean }) => useQuery(key, fetcher, { enabled }),
      { initialProps: { key: null as string | null, enabled: true } },
    )

    expect(result.current.status).toBe('pending')
    expect(result.current.isFetching).toBe(false)
    expect(fetcher).not.toHaveBeenCalled()

    rerender({ key: 'k1', enabled: false })
    expect(result.current.isFetching).toBe(false)
    expect(fetcher).not.toHaveBeenCalled()

    rerender({ key: 'k1', enabled: true })
    await waitFor(() => {
      expect(result.current.status).toBe('success')
    })
    expect(result.current.data).toBe('loaded')
    expect(fetcher).toHaveBeenCalledTimes(1)
  })

  it('resets state and aborts the in-flight request when the key changes', async () => {
    const first = createDeferred<string>()
    const second = createDeferred<string>()
    const signals: AbortSignal[] = []

    const fetcher = vi.fn((signal: AbortSignal) => {
      signals.push(signal)
      return signals.length === 1 ? first.promise : second.promise
    })

    const { result, rerender } = renderHook(({ id }) => useQuery(['consignment', id], fetcher), {
      initialProps: { id: '1' },
    })

    expect(signals).toHaveLength(1)
    expect(signals[0].aborted).toBe(false)

    // Resolve first, then change key while a refetch or next request is tested
    await act(async () => {
      first.resolve('first-value')
      await first.promise
    })
    expect(result.current.data).toBe('first-value')
    expect(result.current.status).toBe('success')

    // Now start a new key with a slow request, then switch key before it resolves
    const third = createDeferred<string>()
    fetcher.mockImplementationOnce((signal: AbortSignal) => {
      signals.push(signal)
      return second.promise
    })
    fetcher.mockImplementationOnce((signal: AbortSignal) => {
      signals.push(signal)
      return third.promise
    })

    rerender({ id: '2' })

    // State resets immediately on key change
    expect(result.current.status).toBe('pending')
    expect(result.current.data).toBeUndefined()
    expect(result.current.error).toBeNull()
    expect(result.current.isFetching).toBe(true)
    expect(signals).toHaveLength(2)
    expect(signals[1].aborted).toBe(false)

    // Switch key again while request 2 is still in flight
    rerender({ id: '3' })
    expect(signals[1].aborted).toBe(true)
    expect(signals).toHaveLength(3)
    expect(signals[2].aborted).toBe(false)

    // Stale response from request 2 is ignored even if it resolves after abort
    await act(async () => {
      second.resolve('stale-second')
      await second.promise
    })
    expect(result.current.data).toBeUndefined()
    expect(result.current.status).toBe('pending')
    expect(result.current.isFetching).toBe(true)

    await act(async () => {
      third.resolve('fresh-third')
      await third.promise
    })
    expect(result.current.status).toBe('success')
    expect(result.current.data).toBe('fresh-third')
    expect(result.current.isFetching).toBe(false)
  })

  it('keeps the last data while a new key loads when keepPreviousData is set', async () => {
    const second = createDeferred<string>()
    const fetcher = vi.fn<(_signal: AbortSignal) => Promise<string>>(() => Promise.resolve('page-1'))

    const { result, rerender } = renderHook(
      ({ page }) => useQuery(['list', page], fetcher, { keepPreviousData: true }),
      { initialProps: { page: 1 } },
    )
    await waitFor(() => expect(result.current.data).toBe('page-1'))

    fetcher.mockImplementationOnce(() => second.promise)
    rerender({ page: 2 })

    expect(result.current.status).toBe('success')
    expect(result.current.data).toBe('page-1')
    expect(result.current.isFetching).toBe(true)

    await act(async () => {
      second.resolve('page-2')
      await second.promise
    })
    expect(result.current.data).toBe('page-2')
    expect(result.current.isFetching).toBe(false)
  })

  it('polls according to refetchInterval and stops when it returns false', async () => {
    vi.useFakeTimers()

    let count = 0
    const fetcher = vi.fn(() => Promise.resolve({ stepCount: ++count }))
    const refetchInterval = vi.fn((data: { stepCount: number } | undefined, fetchCount: number) =>
      (data?.stepCount ?? 0) < 3 && fetchCount < 5 ? 1000 * fetchCount : false,
    )

    const { result } = renderHook(() => useQuery('poll-key', fetcher, { refetchInterval }))

    // Initial fetch (fetchCount = 1)
    await act(async () => {
      await Promise.resolve()
    })

    expect(fetcher).toHaveBeenCalledTimes(1)
    expect(refetchInterval).toHaveBeenLastCalledWith({ stepCount: 1 }, 1)
    expect(result.current.data).toEqual({ stepCount: 1 })
    expect(result.current.polling).toBe(true)
    expect(result.current.isFetching).toBe(false)

    // Advance 1000ms for poll #1 (fetchCount = 2)
    await act(async () => {
      await vi.advanceTimersByTimeAsync(1000)
    })

    expect(fetcher).toHaveBeenCalledTimes(2)
    expect(refetchInterval).toHaveBeenLastCalledWith({ stepCount: 2 }, 2)
    expect(result.current.data).toEqual({ stepCount: 2 })
    expect(result.current.polling).toBe(true)

    // Advance 2000ms for poll #2 (fetchCount = 3) -> stepCount becomes 3, refetchInterval returns false
    await act(async () => {
      await vi.advanceTimersByTimeAsync(2000)
    })

    expect(fetcher).toHaveBeenCalledTimes(3)
    expect(refetchInterval).toHaveBeenLastCalledWith({ stepCount: 3 }, 3)
    expect(result.current.data).toEqual({ stepCount: 3 })
    expect(result.current.polling).toBe(false)
    expect(result.current.isFetching).toBe(false)

    // No further polls scheduled
    await act(async () => {
      await vi.advanceTimersByTimeAsync(10000)
    })
    expect(fetcher).toHaveBeenCalledTimes(3)
  })

  it('keeps polling through transient poll errors until retries are exhausted', async () => {
    vi.useFakeTimers()

    const fetcher = vi
      .fn<(_signal: AbortSignal) => Promise<{ ready: boolean }>>()
      .mockResolvedValueOnce({ ready: false })
      .mockRejectedValueOnce(new Error('network blip'))
      .mockResolvedValueOnce({ ready: true })

    const refetchInterval = vi.fn((data: { ready: boolean } | undefined, fetchCount: number) => {
      if (data?.ready) return false
      return fetchCount < 4 ? 500 : false
    })

    const { result } = renderHook(() => useQuery('transient-poll', fetcher, { refetchInterval }))

    // Initial fetch succeeds with { ready: false }
    await act(async () => {
      await Promise.resolve()
    })
    expect(result.current.data).toEqual({ ready: false })
    expect(result.current.polling).toBe(true)
    expect(result.current.error).toBeNull()

    // Second fetch (poll) fails transiently — keeps last data, stays polling, does not surface error yet
    await act(async () => {
      await vi.advanceTimersByTimeAsync(500)
    })
    expect(fetcher).toHaveBeenCalledTimes(2)
    expect(result.current.data).toEqual({ ready: false })
    expect(result.current.error).toBeNull()
    expect(result.current.polling).toBe(true)

    // Third fetch (poll) succeeds with { ready: true } and stops polling
    await act(async () => {
      await vi.advanceTimersByTimeAsync(500)
    })
    expect(fetcher).toHaveBeenCalledTimes(3)
    expect(result.current.data).toEqual({ ready: true })
    expect(result.current.status).toBe('success')
    expect(result.current.error).toBeNull()
    expect(result.current.polling).toBe(false)
  })

  it('surfaces the error when a failed poll exhausts refetchInterval', async () => {
    vi.useFakeTimers()

    const fetcher = vi
      .fn<(_signal: AbortSignal) => Promise<{ ready: boolean }>>()
      .mockResolvedValueOnce({ ready: false })
      .mockRejectedValueOnce(new Error('persistent failure'))

    const refetchInterval = vi.fn((_data: { ready: boolean } | undefined, fetchCount: number) =>
      fetchCount < 2 ? 500 : false,
    )

    const { result } = renderHook(() => useQuery('exhausted-poll', fetcher, { refetchInterval }))

    await act(async () => {
      await Promise.resolve()
    })
    expect(result.current.polling).toBe(true)

    await act(async () => {
      await vi.advanceTimersByTimeAsync(500)
    })
    expect(result.current.polling).toBe(false)
    expect(result.current.status).toBe('error')
    expect(result.current.error?.message).toBe('persistent failure')
    expect(result.current.data).toEqual({ ready: false })
  })

  it('starts a fresh cycle on refetch, cancelling any pending poll timer or in-flight request', async () => {
    vi.useFakeTimers()

    const fetchCounts: number[] = []
    let call = 0
    const fetcher = vi.fn(() => Promise.resolve(`res-${++call}`))
    const refetchInterval = vi.fn((_data: string | undefined, fetchCount: number) => {
      fetchCounts.push(fetchCount)
      return fetchCount < 2 ? 1000 : false
    })

    const { result } = renderHook(() => useQuery('refetch-key', fetcher, { refetchInterval }))

    await act(async () => {
      await Promise.resolve()
    })
    expect(result.current.data).toBe('res-1')
    expect(result.current.polling).toBe(true)
    expect(fetchCounts).toEqual([1])

    // Before the 1000ms timer fires, call refetch() -> resets fetchCount to 1
    await act(async () => {
      await result.current.refetch()
    })

    expect(result.current.data).toBe('res-2')
    expect(fetchCounts).toEqual([1, 1])
    expect(result.current.polling).toBe(true)

    // Advance 1000ms for the new cycle's poll (fetchCount = 2)
    await act(async () => {
      await vi.advanceTimersByTimeAsync(1000)
    })
    expect(result.current.data).toBe('res-3')
    expect(fetchCounts).toEqual([1, 1, 2])
    expect(result.current.polling).toBe(false)
  })

  it('keeps the last data when a subsequent refetch fails, and restores status to success on the next refetch', async () => {
    const thirdFetch = createDeferred<string>()
    const fetcher = vi
      .fn<(_signal: AbortSignal) => Promise<string>>()
      .mockResolvedValueOnce('initial-data')
      .mockRejectedValueOnce(new Error('refetch failed'))
      .mockImplementationOnce(() => thirdFetch.promise)

    const { result } = renderHook(() => useQuery('keep-data', fetcher))

    await waitFor(() => {
      expect(result.current.status).toBe('success')
    })
    expect(result.current.data).toBe('initial-data')

    await act(async () => {
      await result.current.refetch()
    })

    expect(result.current.status).toBe('error')
    expect(result.current.error?.message).toBe('refetch failed')
    expect(result.current.data).toBe('initial-data')
    expect(result.current.isFetching).toBe(false)

    let refetchPromise!: Promise<string | undefined>
    act(() => {
      refetchPromise = result.current.refetch()
    })

    expect(result.current.status).toBe('success')
    expect(result.current.error).toBeNull()
    expect(result.current.data).toBe('initial-data')
    expect(result.current.isFetching).toBe(true)

    await act(async () => {
      thirdFetch.resolve('updated-data')
      await refetchPromise
    })

    expect(result.current.status).toBe('success')
    expect(result.current.data).toBe('updated-data')
    expect(result.current.isFetching).toBe(false)
  })

  it('cancels active work on unmount when disabled query was started via refetch', async () => {
    vi.useFakeTimers()

    const signals: AbortSignal[] = []
    const fetcher = vi.fn((signal: AbortSignal) => {
      signals.push(signal)
      return Promise.resolve('manual')
    })

    const { result, unmount } = renderHook(() =>
      useQuery('disabled-unmount', fetcher, {
        enabled: false,
        refetchInterval: () => 1000,
      }),
    )

    await act(async () => {
      await result.current.refetch()
    })

    expect(fetcher).toHaveBeenCalledTimes(1)
    expect(result.current.polling).toBe(true)

    unmount()

    await act(async () => {
      await vi.advanceTimersByTimeAsync(5000)
    })

    expect(fetcher).toHaveBeenCalledTimes(1)
  })

  it('reads the latest fetcher and refetchInterval without re-running the effect', async () => {
    const fetcher1 = vi.fn(() => Promise.resolve('from-1'))
    const fetcher2 = vi.fn(() => Promise.resolve('from-2'))

    const { result, rerender } = renderHook(({ fn }) => useQuery('stable-key', fn), { initialProps: { fn: fetcher1 } })

    await waitFor(() => {
      expect(result.current.data).toBe('from-1')
    })
    expect(fetcher1).toHaveBeenCalledTimes(1)

    // Rerendering with a new unmemoized fetcher does not trigger a new fetch on its own
    rerender({ fn: fetcher2 })
    expect(fetcher2).not.toHaveBeenCalled()

    // Calling refetch uses the latest fetcher
    await act(async () => {
      await result.current.refetch()
    })
    expect(fetcher2).toHaveBeenCalledTimes(1)
    expect(result.current.data).toBe('from-2')
  })
})
