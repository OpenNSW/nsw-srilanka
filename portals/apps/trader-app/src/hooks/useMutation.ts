import { useCallback, useEffect, useRef, useState } from 'react'

export interface UseMutationResult<TArgs extends unknown[], TResult> {
  mutate: (...args: TArgs) => Promise<TResult>
  isPending: boolean
  error: Error | null
}

function toError(err: unknown): Error {
  return err instanceof Error ? err : new Error(String(err))
}

export function useMutation<TArgs extends unknown[], TResult>(
  fn: (...args: TArgs) => Promise<TResult>,
): UseMutationResult<TArgs, TResult> {
  const [isPending, setIsPending] = useState(false)
  const [error, setError] = useState<Error | null>(null)
  const fnRef = useRef(fn)
  useEffect(() => {
    fnRef.current = fn
  })
  const callIdRef = useRef(0)
  const pendingCountRef = useRef(0)

  const mutate = useCallback(async (...args: TArgs): Promise<TResult> => {
    const callId = ++callIdRef.current
    pendingCountRef.current += 1
    setIsPending(true)
    setError(null)
    try {
      return await fnRef.current(...args)
    } catch (err) {
      const normalized = toError(err)
      if (callId === callIdRef.current) {
        setError(normalized)
      }
      throw err
    } finally {
      pendingCountRef.current -= 1
      if (pendingCountRef.current === 0) {
        setIsPending(false)
      }
    }
  }, [])

  return { mutate, isPending, error }
}
