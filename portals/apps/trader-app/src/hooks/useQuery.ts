import { useCallback, useEffect, useEffectEvent, useRef, useState } from 'react'

export type QueryKey = string | number | readonly unknown[] | null | undefined

export type QueryStatus = 'pending' | 'success' | 'error'

export interface UseQueryOptions<T> {
  enabled?: boolean
  refetchInterval?: (data: T | undefined, fetchCount: number) => number | false
}

export interface UseQueryResult<T> {
  status: QueryStatus
  data: T | undefined
  error: Error | null
  isFetching: boolean
  polling: boolean
  refetch: () => Promise<T | undefined>
}

function serializeKey(key: QueryKey): string | null {
  if (key === null || key === undefined) return null
  if (typeof key === 'string' || typeof key === 'number') return `${typeof key}:${key}`
  return `array:${JSON.stringify(key)}`
}

function toError(err: unknown): Error {
  return err instanceof Error ? err : new Error(String(err))
}

export function useQuery<T>(
  key: QueryKey,
  fetcher: (signal: AbortSignal) => Promise<T>,
  options?: UseQueryOptions<T>,
): UseQueryResult<T> {
  const serializedKey = serializeKey(key)
  const enabled = (options?.enabled ?? true) && serializedKey !== null

  const [prevKey, setPrevKey] = useState(serializedKey)
  const [prevEnabled, setPrevEnabled] = useState(enabled)
  const [status, setStatus] = useState<QueryStatus>('pending')
  const [data, setData] = useState<T | undefined>(undefined)
  const [error, setError] = useState<Error | null>(null)
  const [isFetching, setIsFetching] = useState<boolean>(enabled)
  const [polling, setPolling] = useState<boolean>(false)

  if (serializedKey !== prevKey) {
    setPrevKey(serializedKey)
    setPrevEnabled(enabled)
    setStatus('pending')
    setData(undefined)
    setError(null)
    setIsFetching(enabled)
    setPolling(false)
  } else if (enabled !== prevEnabled) {
    setPrevEnabled(enabled)
    setIsFetching(enabled)
    if (!enabled) {
      setPolling(false)
    }
  }

  const dataRef = useRef<T | undefined>(undefined)
  const controllerRef = useRef<AbortController | null>(null)
  const timerRef = useRef<ReturnType<typeof setTimeout> | null>(null)
  const runRef = useRef<((fetchCount: number) => Promise<T | undefined>) | null>(null)

  const clearTimer = useCallback(() => {
    if (timerRef.current !== null) {
      clearTimeout(timerRef.current)
      timerRef.current = null
    }
  }, [])

  const cancelActive = useCallback(() => {
    clearTimer()
    if (controllerRef.current) {
      controllerRef.current.abort()
      controllerRef.current = null
    }
  }, [clearTimer])

  const onFetch = useEffectEvent((signal: AbortSignal) => fetcher(signal))
  const onRefetchInterval = useEffectEvent((currentData: T | undefined, fetchCount: number): number | false =>
    options?.refetchInterval ? options.refetchInterval(currentData, fetchCount) : false,
  )

  const execute = useEffectEvent((fetchCount: number): Promise<T | undefined> => {
    clearTimer()
    if (controllerRef.current) {
      controllerRef.current.abort()
    }
    const controller = new AbortController()
    controllerRef.current = controller

    return onFetch(controller.signal).then(
      (result) => {
        if (controller.signal.aborted || controllerRef.current !== controller) {
          return undefined
        }

        dataRef.current = result
        setData(result)
        setStatus('success')
        setError(null)

        const delay = onRefetchInterval(result, fetchCount)
        if (delay !== false && delay >= 0) {
          setPolling(true)
          setIsFetching(false)
          timerRef.current = setTimeout(() => {
            if (!controller.signal.aborted && controllerRef.current === controller) {
              setIsFetching(true)
              void runRef.current?.(fetchCount + 1)
            }
          }, delay)
        } else {
          controllerRef.current = null
          setPolling(false)
          setIsFetching(false)
        }

        return result
      },
      (err: unknown) => {
        if (controller.signal.aborted || controllerRef.current !== controller) {
          return undefined
        }

        const normalized = toError(err)
        // On a failed poll (fetchCount > 1), consult refetchInterval with the last known data so
        // transient errors during polling can keep retrying until retries are exhausted.
        const delay = fetchCount > 1 ? onRefetchInterval(dataRef.current, fetchCount) : false
        if (delay !== false && delay >= 0) {
          setPolling(true)
          setIsFetching(false)
          timerRef.current = setTimeout(() => {
            if (!controller.signal.aborted && controllerRef.current === controller) {
              setIsFetching(true)
              void runRef.current?.(fetchCount + 1)
            }
          }, delay)
        } else {
          controllerRef.current = null
          setError(normalized)
          setStatus('error')
          setPolling(false)
          setIsFetching(false)
        }

        return undefined
      },
    )
  })

  useEffect(() => {
    runRef.current = (fetchCount: number) => execute(fetchCount)
  })

  useEffect(() => {
    dataRef.current = undefined
  }, [serializedKey])

  useEffect(() => {
    if (!enabled) {
      cancelActive()
      return cancelActive
    }

    void execute(1)

    return cancelActive
  }, [serializedKey, enabled, cancelActive])

  const refetch = useCallback(async (): Promise<T | undefined> => {
    if (serializedKey === null) return undefined
    setIsFetching(true)
    setPolling(false)
    setError(null)
    setStatus(dataRef.current === undefined ? 'pending' : 'success')
    return runRef.current ? runRef.current(1) : undefined
  }, [serializedKey])

  return {
    status,
    data,
    error,
    isFetching,
    polling,
    refetch,
  }
}
