import type { ReactNode } from 'react'
import { useQuery } from '@/hooks/useQuery'
import { ProfileContext } from './profileContextCore'
import { getProfile } from './profile'
import { ACCESS_SCOPES } from '@/constants/scopes'
import { useScopes } from '@/hooks/useScopes'

// Loads the signed-in user's profile, for a user granted the scope to read it.
// Others, such as admins, have no profile here.
export function ProfileProvider({ children }: { children: ReactNode }) {
  const canReadProfile = useScopes().hasScope(ACCESS_SCOPES.PROFILE_VIEW)
  const { data, error, isFetching, refetch } = useQuery('profile', getProfile, { enabled: canReadProfile })

  const value = {
    profile: data ?? null,
    isLoading: isFetching,
    error,
    refetch: async () => {
      await refetch()
    },
  }

  return <ProfileContext.Provider value={value}>{children}</ProfileContext.Provider>
}
