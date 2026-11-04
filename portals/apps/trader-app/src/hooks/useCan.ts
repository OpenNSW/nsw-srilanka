import { useMemo } from 'react'
import { useAuth } from 'react-oidc-context'
import { SCOPES } from '@/constants/scopes'

// What the portal lets a user do, and the scope each ability needs. Components
// ask for an ability; only this file knows it comes from the token's scopes.
const ABILITIES = {
  'consignment.view': SCOPES.CONSIGNMENT_READ,
  'consignment.create': SCOPES.CONSIGNMENT_WRITE,
  'consignment.startFlow': SCOPES.CONSIGNMENT_FLOW_WRITE,
  'task.view': SCOPES.TASK_READ,
  'task.act': SCOPES.TASK_WRITE,
  'admin.view': SCOPES.CONSIGNMENT_ADMIN_READ,
  'admin.intervene': SCOPES.CONSIGNMENT_ADMIN_WRITE,
  'profile.view': SCOPES.PROFILE_READ,
} as const

export type Ability = keyof typeof ABILITIES

// The scopes in a token response's space-separated `scope`.
export function grantedScopes(scope: string | undefined): ReadonlySet<string> {
  return new Set((scope ?? '').split(' ').filter(Boolean))
}

// The signed-in user's abilities, for a component that checks several. The
// scopes are the IdP's grant from the token response; when the IdP leaves
// `scope` out, oidc-client-ts takes it to have granted every requested scope.
export function useAbilities(): { can: (ability: Ability) => boolean } {
  const scope = useAuth().user?.scope

  return useMemo(() => {
    const granted = grantedScopes(scope)
    return { can: (ability: Ability) => granted.has(ABILITIES[ability]) }
  }, [scope])
}

export function useCan(ability: Ability): boolean {
  return useAbilities().can(ability)
}
