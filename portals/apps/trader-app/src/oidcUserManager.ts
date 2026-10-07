import { createUserManager, parseExtraQueryParams, parseScopes } from '@opennsw/auth'
import { getEnv } from './runtimeConfig'

const APP_URL = getEnv('APP_URL', window.location.origin)

function getExtraQueryParams(name: string): Record<string, string> {
  try {
    return parseExtraQueryParams(getEnv(name))
  } catch (err) {
    throw new Error(`${name}: ${err instanceof Error ? err.message : String(err)}`, { cause: err })
  }
}

export const userManager = createUserManager({
  authority: getEnv('IDP_BASE_URL', 'https://localhost:8090'),
  clientId: getEnv('IDP_CLIENT_ID', 'TRADER_PORTAL_APP'),
  redirectUri: APP_URL,
  scopes: parseScopes(getEnv('IDP_SCOPES')) ?? ['openid', 'profile', 'email', 'group', 'role'],
  // Named by config rather than hard-coded, so the portal is not tied to one IdP's
  // dialect: ThunderID wants RFC 8707 `resource`, another might want `audience`.
  extraQueryParams: getExtraQueryParams('IDP_EXTRA_QUERY_PARAMS'),
})
