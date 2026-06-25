import { useState, useEffect, useCallback } from 'react'

export interface AsyncState<T> {
  data: T | null
  loading: boolean
  error: Error | null
}

export interface UseAsyncReturn<T> extends AsyncState<T> {
  execute: () => Promise<void>
  reset: () => void
}

/**
 * Custom hook for handling async operations with loading and error states
 * @param asyncFunction - The async function to execute
 * @param immediate - Whether to execute immediately on mount
 */
export function useAsync<T>(asyncFunction: () => Promise<T>, immediate = true): UseAsyncReturn<T> {
  const [state, setState] = useState<AsyncState<T>>({
    data: null,
    loading: immediate,
    error: null,
  })

  const execute = useCallback(async () => {
    setState((prev) => ({ ...prev, loading: true, error: null }))
    try {
      const data = await asyncFunction()
      setState({ data, loading: false, error: null })
    } catch (error) {
      setState({
        data: null,
        loading: false,
        error: error instanceof Error ? error : new Error('An error occurred'),
      })
    }
  }, [asyncFunction])

  const reset = useCallback(() => {
    setState({ data: null, loading: false, error: null })
  }, [])

  useEffect(() => {
    if (!immediate) return
    let cancelled = false
    void asyncFunction()
      .then((data) => {
        if (!cancelled) setState({ data, loading: false, error: null })
      })
      .catch((rawError: unknown) => {
        if (!cancelled) {
          setState({
            data: null,
            loading: false,
            error: rawError instanceof Error ? rawError : new Error('An error occurred'),
          })
        }
      })
    return () => {
      cancelled = true
    }
  }, [asyncFunction, immediate])

  return {
    ...state,
    execute,
    reset,
  }
}
