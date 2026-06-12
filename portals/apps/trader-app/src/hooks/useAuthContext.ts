import { useAuth } from 'react-oidc-context'
import type { Role } from '../services/RoleContext'
import { mapClaimsToRoles } from '../utils/roleMapper'

interface UseAuthContextResult {
  isSignedIn: boolean
  isLoading: boolean
  availableRoles: Role[] | null
  isResolvingRoles: boolean
}

export function useAuthContext(): UseAuthContextResult {
  const auth = useAuth()

  let availableRoles: Role[] | null = null
  if (!auth.isLoading && auth.isAuthenticated && auth.user) {
    try {
      availableRoles = mapClaimsToRoles(auth.user.profile as { groups?: unknown })
    } catch {
      availableRoles = []
    }
  }

  return {
    isSignedIn: auth.isAuthenticated,
    isLoading: auth.isLoading,
    availableRoles,
    isResolvingRoles: false,
  }
}
