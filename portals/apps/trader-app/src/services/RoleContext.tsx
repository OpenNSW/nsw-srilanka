import { useState, type ReactNode } from 'react'
import { RoleContext } from './roleContextCore'
import type { Role } from './roleContextCore'

export type { Role } from './roleContextCore'

interface RoleProviderProps {
  children: ReactNode
  availableRoles?: Role[]
  isLoading?: boolean
}

/**
 * Provides global role state for the application.
 * Decoupled from any specific Auth provider.
 */
export function RoleProvider({ children, availableRoles: initialRoles = [], isLoading = false }: RoleProviderProps) {
  const [role, setRoleState] = useState<Role>(() => {
    const savedRole = localStorage.getItem('user-role') as Role
    if (savedRole && initialRoles.includes(savedRole)) {
      return savedRole
    }
    return initialRoles.length > 0 ? initialRoles[0] : 'trader'
  })
  const [availableRoles, setAvailableRolesState] = useState<Role[]>(initialRoles)

  const setRole = (newRole: Role) => {
    if (availableRoles.includes(newRole)) {
      setRoleState(newRole)
      localStorage.setItem('user-role', newRole)
    }
  }

  const setAvailableRoles = (roles: Role[]) => {
    setAvailableRolesState(roles)
    if (roles.length > 0 && !roles.includes(role)) {
      const fallbackRole = roles[0]
      setRoleState(fallbackRole)
      localStorage.setItem('user-role', fallbackRole)
    }
  }

  return (
    <RoleContext.Provider value={{ role, setRole, availableRoles, setAvailableRoles, isLoading }}>
      {children}
    </RoleContext.Provider>
  )
}
