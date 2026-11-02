import { describe, expect, it } from 'vitest'
import { grantedScopes } from '@/hooks/useScopes'

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
