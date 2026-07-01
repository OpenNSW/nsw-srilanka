import { useMemo } from 'react'
import { useAuth } from 'react-oidc-context'
import type { Role } from '@/services/RoleContext'
import { mapClaimsToRoles } from '@/utils/roleMapper'

interface UseAuthContextResult {
  isSignedIn: boolean
  isLoading: boolean
  availableRoles: Role[] | null
  isResolvingRoles: boolean
}

export function useAuthContext(): UseAuthContextResult {
  const auth = useAuth()

  const availableRoles = useMemo(() => {
    if (!auth.isAuthenticated || !auth.user) {
      return null
    }

    try {
      return mapClaimsToRoles(auth.user.profile as { groups?: unknown })
    } catch {
      return []
    }
  }, [auth.isAuthenticated, auth.user])

  return useMemo(
    () => ({
      isSignedIn: auth.isAuthenticated,
      isLoading: auth.isLoading,
      availableRoles,
      isResolvingRoles: false,
    }),
    [auth.isAuthenticated, auth.isLoading, availableRoles],
  )
}
