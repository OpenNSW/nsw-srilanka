import { fireEvent, render, screen, within } from '@testing-library/react'
import { Theme } from '@radix-ui/themes'
import { MemoryRouter } from 'react-router-dom'
import { afterEach, describe, expect, it, vi } from 'vitest'
import '@/i18n'
import { NavMenu } from '@/components/Layout/NavMenu'
import type { NavItemOrGroup } from '@/navigation/navItems'

// config.js sets this in the app; @/constants reads it as it loads.
vi.hoisted(() => {
  window.__APP_CONFIG__ = { API_BASE_URL: 'http://api.test' }
})

const auth = vi.hoisted(() => ({ scope: '' }))
vi.mock('react-oidc-context', () => ({ useAuth: () => ({ user: { scope: auth.scope } }) }))

// The app's nav has a single destination; a test that needs more sets this.
const nav = vi.hoisted(() => ({ structure: undefined as NavItemOrGroup[] | undefined }))
vi.mock('@/navigation/navItems', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/navigation/navItems')>()
  return {
    ...actual,
    get NAV_STRUCTURE() {
      return nav.structure ?? actual.NAV_STRUCTURE
    },
  }
})

describe('NavMenu', () => {
  it('links the consignments list for a user granted to read consignments', () => {
    auth.scope = 'openid consignment:read'
    render(
      <MemoryRouter>
        <NavMenu />
      </MemoryRouter>,
    )

    expect(screen.getByRole('link', { name: 'Consignments' }).getAttribute('href')).toBe('/consignments')
  })

  it('lists nothing for a user granted none of its destinations', () => {
    auth.scope = 'openid consignment:adminread'
    const { container } = render(
      <MemoryRouter>
        <NavMenu />
      </MemoryRouter>,
    )

    expect(container.innerHTML).toBe('')
  })

  describe('with a group and a second destination', () => {
    afterEach(() => {
      nav.structure = undefined
    })

    const groupedNav = (): NavItemOrGroup[] => [
      {
        labelKey: 'nav.consignments',
        route: { path: '/consignments', ability: 'consignment.view' },
        icon: null,
      },
      {
        labelKey: 'nav.verifiedDocs',
        icon: null,
        items: [{ labelKey: 'nav.cases', route: { path: '/cases', ability: 'admin.view' }, icon: null }],
      },
    ]

    it('lays the list layout out as links, with the group as a labelled run of them', () => {
      auth.scope = 'openid consignment:read consignment:adminread'
      nav.structure = groupedNav()
      render(
        <MemoryRouter>
          <NavMenu layout="list" />
        </MemoryRouter>,
      )

      expect(screen.getByRole('link', { name: 'Consignments' }).getAttribute('href')).toBe('/consignments')
      const group = screen.getByRole('group', { name: 'Verified Docs' })
      expect(within(group).getByRole('link', { name: 'Cases' }).getAttribute('href')).toBe('/cases')
      // Nothing to open: no dropdown trigger, no flyouts.
      expect(screen.queryByRole('button')).toBeNull()
    })

    it('offers the dropdown layout as a menu button', () => {
      auth.scope = 'openid consignment:read consignment:adminread'
      nav.structure = groupedNav()
      render(
        <Theme>
          <MemoryRouter>
            <NavMenu />
          </MemoryRouter>
        </Theme>,
      )

      expect(screen.getByRole('button')).toBeTruthy()
      expect(screen.queryByRole('link')).toBeNull()
    })

    it('marks the open destination as the current page and reports a pick', () => {
      auth.scope = 'openid consignment:read consignment:adminread'
      nav.structure = groupedNav()
      const onNavigate = vi.fn()
      render(
        <MemoryRouter initialEntries={['/cases']}>
          <NavMenu layout="list" onNavigate={onNavigate} />
        </MemoryRouter>,
      )

      expect(screen.getByRole('link', { name: 'Cases' }).getAttribute('aria-current')).toBe('page')
      expect(screen.getByRole('link', { name: 'Consignments' }).getAttribute('aria-current')).toBeNull()
      fireEvent.click(screen.getByRole('link', { name: 'Consignments' }))
      expect(onNavigate).toHaveBeenCalledTimes(1)
    })
  })
})
