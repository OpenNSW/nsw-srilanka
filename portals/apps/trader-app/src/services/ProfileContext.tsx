import type { ReactNode } from 'react'
import { useQuery } from '@/hooks/useQuery'
import { ProfileContext } from './profileContextCore'
import { getProfile } from './profile'
import { useCan } from '@/hooks/useCan'

// Loads the signed-in user's profile when they may view it. Otherwise the query
// key is null, so a user who loses the ability also loses the loaded profile.
export function ProfileProvider({ children }: { children: ReactNode }) {
  const canReadProfile = useCan('profile.view')
  const { data, error, isFetching, refetch } = useQuery(canReadProfile ? 'profile' : null, getProfile)

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
