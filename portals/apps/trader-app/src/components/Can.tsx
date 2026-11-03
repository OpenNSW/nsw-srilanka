import type { ReactNode } from 'react'
import { useCan, type Ability } from '@/hooks/useCan'

// Renders children only for a user who may do the ability I names, and
// fallback otherwise.
export function Can({ I, fallback = null, children }: { I: Ability; fallback?: ReactNode; children: ReactNode }) {
  return <>{useCan(I) ? children : fallback}</>
}
