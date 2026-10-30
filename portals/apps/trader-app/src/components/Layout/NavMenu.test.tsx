import { render, screen } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { describe, expect, it, vi } from 'vitest'
import '@/i18n'
import { NavMenu } from '@/components/Layout/NavMenu'

const auth = vi.hoisted(() => ({ scope: '' }))
vi.mock('react-oidc-context', () => ({ useAuth: () => ({ user: { scope: auth.scope } }) }))

describe('NavMenu', () => {
  it('links the consignments list for a user granted to read consignments', () => {
    auth.scope = 'openid nsw:consignment:read'
    render(
      <MemoryRouter>
        <NavMenu />
      </MemoryRouter>,
    )

    expect(screen.getByRole('link', { name: 'Consignments' }).getAttribute('href')).toBe('/consignments')
  })

  it('lists nothing for a user granted none of its destinations', () => {
    auth.scope = 'openid nsw:consignment:adminread'
    const { container } = render(
      <MemoryRouter>
        <NavMenu />
      </MemoryRouter>,
    )

    expect(container.innerHTML).toBe('')
  })
})
