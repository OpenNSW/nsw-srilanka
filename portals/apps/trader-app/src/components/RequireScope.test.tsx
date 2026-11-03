import { render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { RequireScope } from '@/components/RequireScope'

const auth = vi.hoisted(() => ({ scope: '' }))
vi.mock('react-oidc-context', () => ({ useAuth: () => ({ user: { scope: auth.scope } }) }))

function NewConsignment() {
  return (
    <RequireScope scope="consignment:write" fallback={<p>Restricted</p>}>
      <button>New consignment</button>
    </RequireScope>
  )
}

describe('RequireScope', () => {
  it('renders its children for a user granted the scope', () => {
    auth.scope = 'openid consignment:read consignment:write'
    render(<NewConsignment />)

    expect(screen.getByRole('button', { name: 'New consignment' })).toBeTruthy()
    expect(screen.queryByText('Restricted')).toBeNull()
  })

  it('renders the fallback for a user without the scope', () => {
    auth.scope = 'openid consignment:read consignment:flow:write'
    render(<NewConsignment />)

    expect(screen.queryByRole('button', { name: 'New consignment' })).toBeNull()
    expect(screen.getByText('Restricted')).toBeTruthy()
  })

  it('renders nothing without a fallback', () => {
    auth.scope = ''
    const { container } = render(
      <RequireScope scope="consignment:write">
        <button>New consignment</button>
      </RequireScope>,
    )

    expect(container.innerHTML).toBe('')
  })
})
