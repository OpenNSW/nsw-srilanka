import { describe, expect, it } from 'vitest'
import { firstPermittedPath, permittedNav, type NavItem, type NavItemOrGroup } from '@/navigation/navItems'

const consignments: NavItem = {
  labelKey: 'nav.consignments',
  route: { path: '/consignments', scope: 'nsw:consignment:read' },
  icon: null,
}
const engineStatus: NavItem = {
  labelKey: 'nav.menu',
  route: { path: '/admin', scope: 'nsw:consignment:adminread' },
  icon: null,
}
const adminGroup: NavItemOrGroup = { labelKey: 'nav.menu', icon: null, items: [engineStatus] }
const structure: NavItemOrGroup[] = [consignments, adminGroup]

const granting =
  (...granted: string[]) =>
  (scope: string | undefined) =>
    scope !== undefined && granted.includes(scope)

describe('permittedNav', () => {
  it('keeps only the destinations whose scope is granted', () => {
    expect(permittedNav(structure, granting('nsw:consignment:read'))).toEqual([consignments])
    expect(permittedNav(structure, granting('nsw:consignment:read', 'nsw:consignment:adminread'))).toEqual(structure)
  })

  it('drops a group left with no destinations', () => {
    expect(permittedNav(structure, granting())).toEqual([])
  })
})

describe('firstPermittedPath', () => {
  it('is the first destination the user may open, in nav order', () => {
    expect(firstPermittedPath(structure, granting('nsw:consignment:read', 'nsw:consignment:adminread'))).toBe(
      '/consignments',
    )
    expect(firstPermittedPath(structure, granting('nsw:consignment:adminread'))).toBe('/admin')
  })

  it('is null when the user may open none', () => {
    expect(firstPermittedPath(structure, granting('nsw:task:read'))).toBeNull()
  })
})
