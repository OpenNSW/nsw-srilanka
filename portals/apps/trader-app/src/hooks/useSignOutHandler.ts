import { useCallback } from 'react'
import { useAuth } from 'react-oidc-context'

export function useSignOutHandler(): () => void {
  const auth = useAuth()

  return useCallback(() => {
    void auth.signoutRedirect()
  }, [auth])
}
