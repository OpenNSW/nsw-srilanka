import type { ReactNode } from 'react'
import { useCan, type Ability } from '@/hooks/useCan'

// Renders children for a user with the ability I, and fallback otherwise.
export function Can({ I, fallback = null, children }: { I: Ability; fallback?: ReactNode; children: ReactNode }) {
  return <>{useCan(I) ? children : fallback}</>
}
