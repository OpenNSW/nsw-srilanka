import { renderHook } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { grantedScopes, useAbilities } from '@/hooks/useCan'

const auth = vi.hoisted(() => ({ scope: '' }))
vi.mock('react-oidc-context', () => ({ useAuth: () => ({ user: { scope: auth.scope } }) }))

describe('grantedScopes', () => {
  it('reads the space-separated scopes of a token response', () => {
    expect([...grantedScopes('openid  consignment:read task:read')]).toEqual([
      'openid',
      'consignment:read',
      'task:read',
    ])
  })

  it('grants nothing without a scope', () => {
    expect(grantedScopes(undefined).size).toBe(0)
    expect(grantedScopes('').size).toBe(0)
  })
})

describe('useAbilities', () => {
  it('grants each ability whose scope the token carries', () => {
    auth.scope = 'openid task:read task:write'
    const { result } = renderHook(() => useAbilities())

    expect(result.current.can('task.view')).toBe(true)
    expect(result.current.can('task.act')).toBe(true)
    expect(result.current.can('admin.intervene')).toBe(false)
  })
})
