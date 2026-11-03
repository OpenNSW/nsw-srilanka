import { useMemo } from 'react'
import { useAuth } from 'react-oidc-context'
import { ROLE_CLAIM_NAME, ROLE_LABELS, type RoleLabel } from '@/constants/roles'

export interface UserRole {
  // The role as it appears in the token.
  role: string
  // What the user menu shows: the configured label, else the role itself.
  title: string
  description?: string
}

// The roles in a token claim, in token order. The claim is an array of role
// names; an IdP that has a user with one role may send it as a plain string.
// Anything else holds no roles.
export function rolesFromClaim(value: unknown): string[] {
  const entries = Array.isArray(value) ? value : [value]
  const roles = entries.filter((entry): entry is string => typeof entry === 'string' && entry.trim() !== '')
  return [...new Set(roles)]
}

export function labelRoles(roles: string[], labels: Record<string, RoleLabel>): UserRole[] {
  return roles.map((role) => {
    // hasOwn: a role named like an Object.prototype member has no label.
    const label = Object.hasOwn(labels, role) ? labels[role] : undefined
    return { role, title: label?.title ?? role, description: label?.description }
  })
}

// The roles the IdP put in the signed-in user's ID token, labelled for display.
export function useRoles(): UserRole[] {
  const profile = useAuth().user?.profile

  return useMemo(() => labelRoles(rolesFromClaim(profile?.[ROLE_CLAIM_NAME]), ROLE_LABELS), [profile])
}
