import type { ReactNode } from 'react'
import { useScopes } from '@/hooks/useScopes'

// Renders children only for a user granted scope, and fallback otherwise.
export function RequireScope({
  scope,
  fallback = null,
  children,
}: {
  scope: string
  fallback?: ReactNode
  children: ReactNode
}) {
  const { hasScope } = useScopes()
  return <>{hasScope(scope) ? children : fallback}</>
}
