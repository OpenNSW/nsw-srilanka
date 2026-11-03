import type { ReactNode } from 'react'
import { useQuery } from '@/hooks/useQuery'
import { ProfileContext } from './profileContextCore'
import { getProfile } from './profile'

export function ProfileProvider({ children }: { children: ReactNode }) {
  const { data, error, isFetching, refetch } = useQuery('profile', getProfile)

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
