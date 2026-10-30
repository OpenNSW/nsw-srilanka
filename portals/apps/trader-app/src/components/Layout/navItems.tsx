import { DashboardIcon } from '@radix-ui/react-icons'
import type { ReactNode } from 'react'
import type { ParseKeys } from 'i18next'
import { ROUTES } from '@/constants'
import { ACCESS_SCOPES } from '@/constants/scopes'
import { isAgencyMode } from '@/features/case/mode'

export interface NavItem {
  labelKey: ParseKeys
  path: string
  icon: ReactNode
  // Listed only for a user granted this scope: the one the destination's screen needs.
  scope: string | undefined
}

export interface NavGroup {
  labelKey: ParseKeys
  icon: ReactNode
  items: NavItem[]
}

export type NavItemOrGroup = NavItem | NavGroup

export function isNavGroup(item: NavItemOrGroup): item is NavGroup {
  return 'items' in item
}

// The app's nav destinations, kept as an items/groups structure (not just a
// single link) so a second destination or a grouped section slots in without
// a redesign.
export const NAV_STRUCTURE: NavItemOrGroup[] = [
  {
    labelKey: isAgencyMode ? 'nav.cases' : 'nav.consignments',
    path: ROUTES.CONSIGNMENTS,
    icon: <DashboardIcon className="w-4 h-4" />,
    scope: ACCESS_SCOPES.CONSIGNMENTS_VIEW,
  },
]

// The destinations in structure that a user with these scopes may open. A
// group left with no items is dropped.
export function permittedNav(
  structure: NavItemOrGroup[],
  hasScope: (scope: string | undefined) => boolean,
): NavItemOrGroup[] {
  return structure.flatMap((item): NavItemOrGroup[] => {
    if (!isNavGroup(item)) return hasScope(item.scope) ? [item] : []
    const items = item.items.filter((child) => hasScope(child.scope))
    return items.length > 0 ? [{ ...item, items }] : []
  })
}

// The first destination in nav order that a user with these scopes may open,
// or null when there is none.
export function firstPermittedPath(
  structure: NavItemOrGroup[],
  hasScope: (scope: string | undefined) => boolean,
): string | null {
  const first = permittedNav(structure, hasScope)[0]
  if (!first) return null
  return isNavGroup(first) ? (first.items[0]?.path ?? null) : first.path
}
