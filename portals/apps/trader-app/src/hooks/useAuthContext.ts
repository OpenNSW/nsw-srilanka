import { useMemo } from 'react'
import { useAuth } from 'react-oidc-context'

interface UseAuthContextResult {
  isSignedIn: boolean
  isLoading: boolean
}

export function useAuthContext(): UseAuthContextResult {
  const auth = useAuth()

  return useMemo(
    () => ({
      isSignedIn: auth.isAuthenticated,
      isLoading: auth.isLoading,
    }),
    [auth.isAuthenticated, auth.isLoading],
  )
}
