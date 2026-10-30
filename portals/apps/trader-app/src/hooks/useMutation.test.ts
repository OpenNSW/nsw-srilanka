import { act, renderHook } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { useMutation } from '@/hooks/useMutation'

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

describe('useMutation', () => {
  it('tracks isPending and resolves with the mutation result', async () => {
    const deferred = createDeferred<{ id: string }>()
    const fn = vi.fn<(_name: string) => Promise<{ id: string }>>(() => deferred.promise)

    const { result } = renderHook(() => useMutation(fn))

    expect(result.current.isPending).toBe(false)
    expect(result.current.error).toBeNull()

    let promise!: Promise<{ id: string }>
    act(() => {
      promise = result.current.mutate('test-consignment')
    })

    expect(result.current.isPending).toBe(true)
    expect(result.current.error).toBeNull()
    expect(fn).toHaveBeenCalledWith('test-consignment')

    let resolvedValue: { id: string } | undefined
    await act(async () => {
      deferred.resolve({ id: 'c-123' })
      resolvedValue = await promise
    })

    expect(resolvedValue).toEqual({ id: 'c-123' })
    expect(result.current.isPending).toBe(false)
    expect(result.current.error).toBeNull()
  })

  it('captures the error, rethrows it to the caller, and clears it on the next call', async () => {
    const fn = vi
      .fn<(_input: string) => Promise<string>>()
      .mockRejectedValueOnce(new Error('save failed'))
      .mockResolvedValueOnce('saved')

    const { result } = renderHook(() => useMutation(fn))

    await act(async () => {
      await expect(result.current.mutate('first')).rejects.toThrow('save failed')
    })

    expect(result.current.isPending).toBe(false)
    expect(result.current.error?.message).toBe('save failed')

    await act(async () => {
      const value = await result.current.mutate('second')
      expect(value).toBe('saved')
    })

    expect(result.current.isPending).toBe(false)
    expect(result.current.error).toBeNull()
  })

  it('always calls the latest mutation function without changing mutate identity', async () => {
    const fn1 = vi.fn(() => Promise.resolve('one'))
    const fn2 = vi.fn(() => Promise.resolve('two'))

    const { result, rerender } = renderHook(({ fn }) => useMutation(fn), {
      initialProps: { fn: fn1 },
    })

    const initialMutate = result.current.mutate
    rerender({ fn: fn2 })

    expect(result.current.mutate).toBe(initialMutate)

    await act(async () => {
      const res = await result.current.mutate()
      expect(res).toBe('two')
    })
    expect(fn1).not.toHaveBeenCalled()
    expect(fn2).toHaveBeenCalledTimes(1)
  })

  it('keeps isPending true when overlapping calls settle out of order', async () => {
    const first = createDeferred<string>()
    const second = createDeferred<string>()
    let call = 0
    const fn = vi.fn(() => (++call === 1 ? first.promise : second.promise))

    const { result } = renderHook(() => useMutation(fn))

    let firstPromise!: Promise<string>
    let secondPromise!: Promise<string>
    act(() => {
      firstPromise = result.current.mutate()
      secondPromise = result.current.mutate()
    })

    expect(result.current.isPending).toBe(true)

    // Newer call finishes first; older call is still in flight
    await act(async () => {
      second.resolve('second-done')
      await secondPromise
    })
    expect(result.current.isPending).toBe(true)

    // Older call finishes; now all calls have settled
    await act(async () => {
      first.resolve('first-done')
      await firstPromise
    })
    expect(result.current.isPending).toBe(false)
  })
})
