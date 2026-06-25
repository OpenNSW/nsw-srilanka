import { createContext } from 'react'

export type Role = 'trader' | 'cha'

export interface RoleContextType {
  role: Role
  setRole: (role: Role) => void
  availableRoles: Role[]
  setAvailableRoles: (roles: Role[]) => void
  isLoading: boolean
}

export const RoleContext = createContext<RoleContextType | undefined>(undefined)
