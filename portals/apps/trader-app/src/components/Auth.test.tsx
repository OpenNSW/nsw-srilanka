import { fireEvent, render, screen } from '@testing-library/react'
import { Theme } from '@radix-ui/themes'
import { beforeAll, beforeEach, describe, expect, it, vi } from 'vitest'
import i18n from '@/i18n'
import { UserDropdown } from '@/components/Auth'

const auth = vi.hoisted((): { profile: Record<string, unknown> } => ({ profile: {} }))

vi.mock('react-oidc-context', () => ({
  useAuth: () => ({ user: { profile: auth.profile } }),
}))
vi.mock('@/constants/roles', () => ({
  ROLE_CLAIM_NAME: 'roles',
  ROLE_LABELS: { Trader: { title: 'Trader', description: 'Creates and manages consignments.' } },
}))

// jsdom has no ResizeObserver, which the menu's popper positioning needs.
class ResizeObserverStub {
  observe() {}
  unobserve() {}
  disconnect() {}
}

function renderOpenMenu(onSignOut = vi.fn()) {
  render(
    <Theme>
      <UserDropdown onSignOut={onSignOut} />
    </Theme>,
  )
  fireEvent.pointerDown(screen.getByRole('button'), { button: 0, ctrlKey: false })
  return onSignOut
}

describe('UserDropdown', () => {
  beforeAll(async () => {
    vi.stubGlobal('ResizeObserver', ResizeObserverStub)
    await i18n.changeLanguage('en')
  })

  beforeEach(() => {
    auth.profile = { name: 'Suresh Fernando', email: 'suresh@example.com' }
  })

  it('shows the name and email of the signed-in user', () => {
    renderOpenMenu()

    expect(screen.getByText('Suresh Fernando')).toBeTruthy()
    expect(screen.getByText('suresh@example.com')).toBeTruthy()
  })

  it('lists each role under "Roles" with its configured title and description', () => {
    auth.profile = { ...auth.profile, roles: ['Trader', 'CDA Officer'] }
    renderOpenMenu()

    expect(screen.getByText('Roles')).toBeTruthy()
    expect(screen.getByText('Trader')).toBeTruthy()
    expect(screen.getByText('Creates and manages consignments.')).toBeTruthy()
    // A role with no label is shown by its own name.
    expect(screen.getByText('CDA Officer')).toBeTruthy()
  })

  it('leaves out the roles section when the token has no roles', () => {
    renderOpenMenu()

    expect(screen.queryByText('Roles')).toBeNull()
  })

  it('signs the user out', () => {
    const onSignOut = renderOpenMenu()

    fireEvent.click(screen.getByRole('menuitem', { name: 'Sign out' }))

    expect(onSignOut).toHaveBeenCalledOnce()
  })
})
