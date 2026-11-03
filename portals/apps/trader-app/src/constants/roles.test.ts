import { afterEach, describe, expect, it, vi } from 'vitest'
import { parseRoleLabels } from '@/constants/roles'

describe('parseRoleLabels', () => {
  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('maps each role to its title and optional description', () => {
    const labels = parseRoleLabels(
      JSON.stringify({
        Trader: { title: 'Trader', description: 'Creates consignments.' },
        CHA: { title: 'Customs House Agent' },
      }),
    )

    expect(labels).toEqual({
      Trader: { title: 'Trader', description: 'Creates consignments.' },
      CHA: { title: 'Customs House Agent' },
    })
  })

  it('has no labels when the key is unset or empty', () => {
    expect(parseRoleLabels(undefined)).toEqual({})
    expect(parseRoleLabels('')).toEqual({})
  })

  it.each([
    ['JSON that does not parse', '{Trader: '],
    ['a list', '[{"title": "Trader"}]'],
    ['a role given as a plain string', '{"Trader": "Trader"}'],
    ['a role without a title', '{"Trader": {"description": "Creates consignments."}}'],
    ['a role with a blank title', '{"Trader": {"title": "  "}}'],
    ['a description that is not text', '{"Trader": {"title": "Trader", "description": 3}}'],
  ])('ignores every label and says so for %s', (_, raw) => {
    const error = vi.spyOn(console, 'error').mockImplementation(() => {})

    expect(parseRoleLabels(raw)).toEqual({})
    expect(error).toHaveBeenCalledOnce()
    expect(error.mock.calls[0][0]).toContain('IDP_ROLE_LABELS')
  })
})

describe('role config', () => {
  afterEach(() => {
    window.__APP_CONFIG__ = {}
    vi.resetModules()
  })

  it('reads the roles from the "roles" claim, with no labels, when nothing is set', async () => {
    window.__APP_CONFIG__ = {}
    const { ROLE_CLAIM_NAME, ROLE_LABELS } = await import('@/constants/roles')

    expect(ROLE_CLAIM_NAME).toBe('roles')
    expect(ROLE_LABELS).toEqual({})
  })

  it('reads the claim name and the labels from the runtime config', async () => {
    window.__APP_CONFIG__ = {
      IDP_ROLE_CLAIM_NAME: ' groups ',
      IDP_ROLE_LABELS: '{"NSW Admin": {"title": "NSW Administrator"}}',
    }
    const { ROLE_CLAIM_NAME, ROLE_LABELS } = await import('@/constants/roles')

    expect(ROLE_CLAIM_NAME).toBe('groups')
    expect(ROLE_LABELS).toEqual({ 'NSW Admin': { title: 'NSW Administrator' } })
  })

  it('falls back to the "roles" claim for a blank claim name', async () => {
    window.__APP_CONFIG__ = { IDP_ROLE_CLAIM_NAME: '   ' }
    const { ROLE_CLAIM_NAME } = await import('@/constants/roles')

    expect(ROLE_CLAIM_NAME).toBe('roles')
  })
})
