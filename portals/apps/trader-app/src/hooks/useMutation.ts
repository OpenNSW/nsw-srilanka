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

  const mutate = useCallback(async (...args: TArgs): Promise<TResult> => {
    const callId = ++callIdRef.current
    setIsPending(true)
    setError(null)
    try {
      const result = await fnRef.current(...args)
      if (callId === callIdRef.current) {
        setIsPending(false)
      }
      return result
    } catch (err) {
      const normalized = toError(err)
      if (callId === callIdRef.current) {
        setError(normalized)
        setIsPending(false)
      }
      throw err
    }
  }, [])

  return { mutate, isPending, error }
}
