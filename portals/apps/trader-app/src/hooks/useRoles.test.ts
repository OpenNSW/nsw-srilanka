import { renderHook } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { labelRoles, rolesFromClaim, useRoles } from '@/hooks/useRoles'

const config = vi.hoisted(() => ({ claim: 'roles' }))
const auth = vi.hoisted(() => ({ profile: undefined as Record<string, unknown> | undefined }))

vi.mock('@/constants/roles', () => ({
  get ROLE_CLAIM_NAME() {
    return config.claim
  },
  ROLE_LABELS: { Trader: { title: 'Trader', description: 'Creates consignments.' } },
}))
vi.mock('react-oidc-context', () => ({
  useAuth: () => ({ user: auth.profile && { profile: auth.profile } }),
}))

describe('rolesFromClaim', () => {
  it('lists the roles of an array in token order, once each', () => {
    expect(rolesFromClaim(['Trader', 'CHA', 'Trader'])).toEqual(['Trader', 'CHA'])
  })

  it('takes a plain string as one role', () => {
    expect(rolesFromClaim('NSW Admin')).toEqual(['NSW Admin'])
  })

  it('skips an entry that is not a name', () => {
    expect(rolesFromClaim(['Trader', 7, null, '', '  ', { role: 'CHA' }])).toEqual(['Trader'])
  })

  it.each([undefined, null, '', 3, true, { Trader: true }])('holds no roles for %j', (value) => {
    expect(rolesFromClaim(value)).toEqual([])
  })
})

describe('labelRoles', () => {
  const labels = { Trader: { title: 'Trader', description: 'Creates consignments.' }, CHA: { title: 'Agent' } }

  it('gives a role its configured title and description', () => {
    expect(labelRoles(['Trader', 'CHA'], labels)).toEqual([
      { role: 'Trader', title: 'Trader', description: 'Creates consignments.' },
      { role: 'CHA', title: 'Agent', description: undefined },
    ])
  })

  it('shows a role that has no label by its own name', () => {
    expect(labelRoles(['CDA Officer'], labels)).toEqual([
      { role: 'CDA Officer', title: 'CDA Officer', description: undefined },
    ])
  })

  it('does not take a label from Object.prototype', () => {
    expect(labelRoles(['constructor', '__proto__'], labels).map((r) => r.title)).toEqual(['constructor', '__proto__'])
  })
})

describe('useRoles', () => {
  beforeEach(() => {
    config.claim = 'roles'
    auth.profile = undefined
  })

  it('labels the roles in the ID token', () => {
    auth.profile = { roles: ['Trader', 'CDA Officer'] }
    const { result } = renderHook(() => useRoles())

    expect(result.current).toEqual([
      { role: 'Trader', title: 'Trader', description: 'Creates consignments.' },
      { role: 'CDA Officer', title: 'CDA Officer', description: undefined },
    ])
  })

  it('reads the claim the deployment names', () => {
    config.claim = 'groups'
    auth.profile = { roles: ['Trader'], groups: ['CHA'] }
    const { result } = renderHook(() => useRoles())

    expect(result.current.map((r) => r.role)).toEqual(['CHA'])
  })

  it('has no roles when the token has no such claim or nobody is signed in', () => {
    auth.profile = { email: 'suresh@example.com' }
    expect(renderHook(() => useRoles()).result.current).toEqual([])

    auth.profile = undefined
    expect(renderHook(() => useRoles()).result.current).toEqual([])
  })
})
