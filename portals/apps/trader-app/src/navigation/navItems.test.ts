import { describe, expect, it, vi } from 'vitest'
import type { Ability } from '@/hooks/useCan'
import { firstPermittedPath, permittedNav, type NavItem, type NavItemOrGroup } from '@/navigation/navItems'

// config.js sets this in the app; @/constants reads it as it loads.
vi.hoisted(() => {
  window.__APP_CONFIG__ = { API_BASE_URL: 'http://api.test' }
})

const consignments: NavItem = {
  labelKey: 'nav.consignments',
  route: { path: '/consignments', ability: 'consignment.view' },
  icon: null,
}
const engineStatus: NavItem = {
  labelKey: 'nav.menu',
  route: { path: '/admin', ability: 'admin.view' },
  icon: null,
}
const adminGroup: NavItemOrGroup = { labelKey: 'nav.menu', icon: null, items: [engineStatus] }
const structure: NavItemOrGroup[] = [consignments, adminGroup]

const granting =
  (...granted: Ability[]) =>
  (ability: Ability) =>
    granted.includes(ability)

describe('permittedNav', () => {
  it('keeps only the destinations whose ability is granted', () => {
    expect(permittedNav(structure, granting('consignment.view'))).toEqual([consignments])
    expect(permittedNav(structure, granting('consignment.view', 'admin.view'))).toEqual(structure)
  })

  it('drops a group left with no destinations', () => {
    expect(permittedNav(structure, granting())).toEqual([])
  })
})

describe('firstPermittedPath', () => {
  it('is the first destination the user may open, in nav order', () => {
    expect(firstPermittedPath(structure, granting('consignment.view', 'admin.view'))).toBe('/consignments')
    expect(firstPermittedPath(structure, granting('admin.view'))).toBe('/admin')
  })

  it('is null when the user may open none', () => {
    expect(firstPermittedPath(structure, granting('task.view'))).toBeNull()
  })
})
