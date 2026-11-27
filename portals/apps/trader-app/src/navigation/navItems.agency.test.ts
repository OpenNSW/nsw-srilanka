import { describe, expect, it, vi } from 'vitest'
import type { Ability } from '@/hooks/useCan'
import { NAV_STRUCTURE, homePath, isNavGroup, permittedNav } from '@/navigation/navItems'

// An agency deployment searches cases. isAgencyMode is read once as the nav module loads, so
// this case gets a file of its own.
vi.mock('@/features/case/mode', () => ({ isAgencyMode: true }))

// config.js sets this in the app; @/constants reads it as it loads.
vi.hoisted(() => {
  window.__APP_CONFIG__ = { API_BASE_URL: 'http://api.test' }
})

const granting =
  (...granted: Ability[]) =>
  (ability: Ability) =>
    granted.includes(ability)

describe('the nav in an agency', () => {
  it('leaves the admin dashboard out of the nav, even for an admin', () => {
    const paths = permittedNav(NAV_STRUCTURE, granting('consignment.view', 'admin.view')).flatMap((item) =>
      isNavGroup(item) ? item.items.map((child) => child.route.path) : [item.route.path],
    )
    expect(paths).toEqual(['/cases'])
  })

  it('sends an admin to the admin dashboard and anyone else to the cases', () => {
    // An agency's admin role grants only the admin scopes (onboarding-agency-mode-agencies.md).
    expect(homePath(NAV_STRUCTURE, granting('admin.view'))).toBe('/admin')
    expect(homePath(NAV_STRUCTURE, granting('consignment.view', 'admin.view'))).toBe('/admin')
    expect(homePath(NAV_STRUCTURE, granting('consignment.view'))).toBe('/cases')
  })
})
