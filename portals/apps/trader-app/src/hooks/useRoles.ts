import { useMemo } from 'react'
import { useAuth } from 'react-oidc-context'
import { ROLE_CLAIM_NAME, ROLE_LABELS, type RoleLabel } from '@/constants/roles'

export interface UserRole {
  // The role as it appears in the token.
  role: string
  // What the user menu shows: the role's configured title.
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

// The roles that have a label, in token order, each with its title and
// description. The IdP may put the user's roles for other applications in the
// claim too; the labels name the roles this deployment shows, so a role with
// no label is left out.
export function labelRoles(roles: string[], labels: Record<string, RoleLabel>): UserRole[] {
  return roles.flatMap((role) => {
    // hasOwn: a role named like an Object.prototype member has no label.
    if (!Object.hasOwn(labels, role)) {
      return []
    }
    const { title, description } = labels[role]
    return [{ role, title, description }]
  })
}

// The roles in the signed-in user's ID token that this deployment labels.
export function useRoles(): UserRole[] {
  const profile = useAuth().user?.profile

  return useMemo(() => labelRoles(rolesFromClaim(profile?.[ROLE_CLAIM_NAME]), ROLE_LABELS), [profile])
}
