import { DashboardIcon } from '@radix-ui/react-icons'
import type { ReactNode } from 'react'
import type { ParseKeys } from 'i18next'
import { isAgencyMode } from '@/features/case/mode'
import type { Ability } from '@/hooks/useCan'
import { APP_ROUTES, type AppRoute } from '@/navigation/routes'

export interface NavItem {
  labelKey: ParseKeys
  route: AppRoute
  icon: ReactNode
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
    route: APP_ROUTES.consignments,
    icon: <DashboardIcon className="w-4 h-4" />,
  },
]

// The destinations in structure whose route's ability can permits. A group
// left with no items is dropped.
export function permittedNav(structure: NavItemOrGroup[], can: (ability: Ability) => boolean): NavItemOrGroup[] {
  return structure.flatMap((item): NavItemOrGroup[] => {
    if (!isNavGroup(item)) return can(item.route.ability) ? [item] : []
    const items = item.items.filter((child) => can(child.route.ability))
    return items.length > 0 ? [{ ...item, items }] : []
  })
}

// The path of the first permitted destination in nav order, or null when there
// is none.
export function firstPermittedPath(structure: NavItemOrGroup[], can: (ability: Ability) => boolean): string | null {
  const first = permittedNav(structure, can)[0]
  if (!first) return null
  return isNavGroup(first) ? first.items[0].route.path : first.route.path
}
