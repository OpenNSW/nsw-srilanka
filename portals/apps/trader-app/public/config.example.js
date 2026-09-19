// Local dev runtime config. Copy this file to config.js once per clone
// (config.js is gitignored) — this is the single source of truth for local
// dev, used both by `vite dev` (served as-is from public/) and by
// `docker compose` (bind-mounted into the container — see compose.yml).
// A real deployment supplies its own values via Helm (see
// deployments/helm/values-example.yaml). IDP_SCOPES must include the
// nsw:* API scopes, or the backend rejects the resulting token.
window.__APP_CONFIG__ = {
  API_BASE_URL: 'http://localhost:8080',
  IDP_BASE_URL: 'https://localhost:8090',
  IDP_CLIENT_ID: 'TRADER_PORTAL_APP',
  IDP_EXTRA_QUERY_PARAMS: 'resource=https://api.nsw-srilanka.local',
  APP_URL: 'http://localhost:5173',
  IDP_SCOPES:
    'openid,profile,email,group,role,ou,nsw:consignment:read,nsw:consignment:write,nsw:task:read,nsw:task:write,nsw:hscode:read,nsw:company:read,nsw:cha:read,nsw:storage:read,nsw:storage:write,nsw:profile:read',
  IDP_TRADER_GROUP_NAME: 'Traders',
  IDP_CHA_GROUP_NAME: 'CHA',
  SHOW_AUTOFILL_BUTTON: 'true',
  DEV_ENABLE_TEST_FLOW: 'true',
}
