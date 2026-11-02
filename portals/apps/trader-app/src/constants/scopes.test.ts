import { afterEach, describe, expect, it, vi } from 'vitest'
import { TEST_APP_CONFIG } from '@/test/config'

describe('ACCESS_SCOPES', () => {
  afterEach(() => {
    window.__APP_CONFIG__ = { ...TEST_APP_CONFIG }
    vi.resetModules()
  })

  it('reads the scope of each screen and action from the runtime config', async () => {
    window.__APP_CONFIG__ = { ...TEST_APP_CONFIG, SCOPE_CONSIGNMENTS_CREATE: ' consignment:create ' }
    const { ACCESS_SCOPES } = await import('@/constants/scopes')

    expect(ACCESS_SCOPES.CONSIGNMENTS_CREATE).toBe('consignment:create')
    expect(ACCESS_SCOPES.CONSIGNMENTS_VIEW).toBe('consignment:read')
  })

  it('leaves a key the deployment does not set without a scope', async () => {
    window.__APP_CONFIG__ = { ...TEST_APP_CONFIG, SCOPE_ADMIN_VIEW: undefined, SCOPE_ADMIN_ACTIONS: '' }
    const { ACCESS_SCOPES } = await import('@/constants/scopes')

    expect(ACCESS_SCOPES.ADMIN_VIEW).toBeUndefined()
    expect(ACCESS_SCOPES.ADMIN_ACTIONS).toBeUndefined()
  })
})
