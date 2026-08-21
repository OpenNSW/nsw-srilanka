import { createContext } from 'react'
import type { UserProfile } from './profile'

export interface ProfileContextType {
  profile: UserProfile | null
  isLoading: boolean
  error: Error | null
  refetch: () => Promise<void>
}

export const ProfileContext = createContext<ProfileContextType | undefined>(undefined)
