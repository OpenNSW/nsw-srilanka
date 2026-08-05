import { UserManager, WebStorageStateStore } from 'oidc-client-ts'
import { getEnv } from './runtimeConfig'

const APP_URL = getEnv('VITE_APP_URL', window.location.origin)
const CLIENT_ID = getEnv('VITE_IDP_CLIENT_ID', 'TRADER_PORTAL_APP')
const IDP_BASE_URL = getEnv('VITE_IDP_BASE_URL', 'https://localhost:8090')
const rawScopes = getEnv('VITE_IDP_SCOPES')
const IDP_SCOPES = rawScopes
  ? rawScopes
      .split(',')
      .map((s: string) => s.trim())
      .join(' ')
  : 'openid profile email group'

// RFC 8707 resource indicator: names the resource server this access token is for.
// Required on the authorization request; its identifier becomes the token's `aud`.
const IDP_RESOURCE = getEnv('VITE_IDP_RESOURCE', 'https://api.nsw-srilanka.local')

export const userManager = new UserManager({
  authority: IDP_BASE_URL,
  client_id: CLIENT_ID,
  redirect_uri: APP_URL,
  post_logout_redirect_uri: APP_URL,
  scope: IDP_SCOPES,
  extraQueryParams: { resource: IDP_RESOURCE },
  userStore: new WebStorageStateStore({ store: window.sessionStorage }),
  automaticSilentRenew: true,
})
