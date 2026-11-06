import { fireEvent, render, screen } from '@testing-library/react'
import { Theme } from '@radix-ui/themes'
import { MemoryRouter, Route, Routes, useLocation } from 'react-router-dom'
import { describe, expect, it, vi } from 'vitest'
import '@/i18n'
import { AccessRestrictedScreen } from '@/screens/AccessRestrictedScreen'

// config.js sets this in the app; @/constants reads it as it loads.
vi.hoisted(() => {
  window.__APP_CONFIG__ = { API_BASE_URL: 'http://api.test' }
})

const auth = vi.hoisted(() => ({ scope: '' }))
vi.mock('react-oidc-context', () => ({ useAuth: () => ({ user: { scope: auth.scope } }) }))
vi.mock('@/hooks/useSignOutHandler', () => ({ useSignOutHandler: () => vi.fn() }))

function CurrentPath() {
  return <div data-testid="path">{useLocation().pathname}</div>
}

function renderScreen(scope: string) {
  auth.scope = scope
  render(
    <Theme>
      <MemoryRouter>
        <Routes>
          <Route path="/" element={<AccessRestrictedScreen />} />
          <Route path="*" element={<CurrentPath />} />
        </Routes>
      </MemoryRouter>
    </Theme>,
  )
}

describe('AccessRestrictedScreen', () => {
  it('tells a user granted no screen that their account lacks access', () => {
    renderScreen('openid profile:read')

    expect(screen.getByRole('heading', { name: 'Access Restricted' })).toBeTruthy()
    expect(screen.queryByText('Welcome, administrator')).toBeNull()
  })

  it.each(['consignment:adminread', 'consignment:adminwrite'])('greets a user granted %s as an admin', (scope) => {
    renderScreen(`openid ${scope}`)

    expect(screen.getByRole('heading', { name: 'Welcome, administrator' })).toBeTruthy()
    expect(screen.queryByText('Access Restricted')).toBeNull()
  })

  it('offers sign out in both cases', () => {
    renderScreen('openid consignment:adminread')

    expect(screen.getByRole('button', { name: 'Sign out' })).toBeTruthy()
  })

  it('asks an admin to enter the consignment ID', () => {
    renderScreen('openid consignment:adminread')

    expect(screen.getByText('Please enter the consignment ID to step in.')).toBeTruthy()
  })

  it('shows no ID prompt to a user granted no admin scope', () => {
    renderScreen('openid profile:read')

    expect(screen.queryByText('Please enter the consignment ID to step in.')).toBeNull()
  })

  it('opens the admin view of the entered consignment ID', () => {
    renderScreen('openid consignment:adminread')

    fireEvent.change(screen.getByPlaceholderText('Consignment ID'), { target: { value: ' abc-123 ' } })
    fireEvent.click(screen.getByRole('button', { name: 'Open' }))

    expect(screen.getByTestId('path').textContent).toBe('/admin/consignments/abc-123')
  })

  it('requires an ID before Open can submit', () => {
    renderScreen('openid consignment:adminread')

    expect(screen.getByPlaceholderText('Consignment ID').hasAttribute('required')).toBe(true)
    expect(screen.getByRole('button', { name: 'Open' }).hasAttribute('disabled')).toBe(false)
  })

  it('offers no ID input to a user who cannot read the admin views', () => {
    renderScreen('openid consignment:adminwrite')

    expect(screen.queryByPlaceholderText('Consignment ID')).toBeNull()
  })

  it('offers no ID input to a user granted no admin scope', () => {
    renderScreen('openid profile:read')

    expect(screen.queryByPlaceholderText('Consignment ID')).toBeNull()
  })
})
