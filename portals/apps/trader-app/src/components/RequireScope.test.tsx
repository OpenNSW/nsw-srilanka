import { render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { RequireScope } from '@/components/RequireScope'

const auth = vi.hoisted(() => ({ scope: '' }))
vi.mock('react-oidc-context', () => ({ useAuth: () => ({ user: { scope: auth.scope } }) }))

function NewConsignment() {
  return (
    <RequireScope scope="nsw:consignment:write" fallback={<p>Restricted</p>}>
      <button>New consignment</button>
    </RequireScope>
  )
}

describe('RequireScope', () => {
  it('renders its children for a user granted the scope', () => {
    auth.scope = 'openid nsw:consignment:read nsw:consignment:write'
    render(<NewConsignment />)

    expect(screen.getByRole('button', { name: 'New consignment' })).toBeTruthy()
    expect(screen.queryByText('Restricted')).toBeNull()
  })

  it('renders the fallback for a user without the scope', () => {
    auth.scope = 'openid nsw:consignment:read nsw:consignment:flow'
    render(<NewConsignment />)

    expect(screen.queryByRole('button', { name: 'New consignment' })).toBeNull()
    expect(screen.getByText('Restricted')).toBeTruthy()
  })

  it('renders the fallback when the deployment sets no scope for it', () => {
    auth.scope = 'openid nsw:consignment:read nsw:consignment:write'
    render(
      <RequireScope scope={undefined} fallback={<p>Restricted</p>}>
        <button>New consignment</button>
      </RequireScope>,
    )

    expect(screen.queryByRole('button', { name: 'New consignment' })).toBeNull()
    expect(screen.getByText('Restricted')).toBeTruthy()
  })

  it('renders nothing without a fallback', () => {
    auth.scope = ''
    const { container } = render(
      <RequireScope scope="nsw:consignment:write">
        <button>New consignment</button>
      </RequireScope>,
    )

    expect(container.innerHTML).toBe('')
  })
})
