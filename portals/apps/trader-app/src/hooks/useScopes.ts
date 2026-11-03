import { useMemo } from 'react'
import { useAuth } from 'react-oidc-context'

interface UseScopesResult {
  hasScope: (scope: string) => boolean
}

// The scopes in a token response's space-separated `scope`.
export function grantedScopes(scope: string | undefined): ReadonlySet<string> {
  return new Set((scope ?? '').split(' ').filter(Boolean))
}

// The scopes the IdP granted the signed-in user, from the token response. When
// the IdP leaves `scope` out, oidc-client-ts takes it to have granted every
// requested scope.
export function useScopes(): UseScopesResult {
  const scope = useAuth().user?.scope

  return useMemo(() => {
    const granted = grantedScopes(scope)
    return { hasScope: (s: string) => granted.has(s) }
  }, [scope])
}
