// Local dev runtime config. Copy this file to config.js once per clone
// (config.js is gitignored) — this is the single source of truth for local
// dev, used both by `vite dev` (served as-is from public/) and by
// `docker compose` (bind-mounted into the container — see compose.yml).
// A real deployment supplies its own values via Helm (see
// deployments/helm/values-example.yaml). IDP_SCOPES must include the
// nsw:* API scopes: the backend checks them on every call, and the portal
// shows a screen or action only to a user the IdP granted its scope.
window.__APP_CONFIG__ = {
  API_BASE_URL: 'http://localhost:8080',
  IDP_BASE_URL: 'http://localhost:8090',
  IDP_CLIENT_ID: 'TRADER_PORTAL_APP',
  IDP_EXTRA_QUERY_PARAMS: 'resource=https://api.nsw-srilanka.local',
  APP_URL: 'http://localhost:5173',
  IDP_SCOPES:
    'openid,profile,email,group,role,ou,nsw:consignment:read,nsw:consignment:write,nsw:consignment:flow:write,nsw:task:read,nsw:task:write,nsw:hscode:read,nsw:company:read,nsw:cha:read,nsw:storage:read,nsw:storage:write,nsw:profile:read,nsw:consignment:adminread,nsw:consignment:adminwrite',
  // The scope each screen and action needs: the portal shows it only to a user
  // the IdP granted that scope. A SCOPE_* key left unset hides its screen or
  // action from everyone.
  SCOPE_CONSIGNMENTS_VIEW: 'nsw:consignment:read',
  SCOPE_CONSIGNMENTS_CREATE: 'nsw:consignment:write',
  SCOPE_CONSIGNMENT_FLOWS: 'nsw:consignment:flow:write',
  SCOPE_TASKS_VIEW: 'nsw:task:read',
  SCOPE_TASKS_ACTIONS: 'nsw:task:write',
  SCOPE_ADMIN_VIEW: 'nsw:consignment:adminread',
  SCOPE_ADMIN_ACTIONS: 'nsw:consignment:adminwrite',
  SCOPE_PROFILE_VIEW: 'nsw:profile:read',
  SHOW_AUTOFILL_BUTTON: 'true',
  // Shows the demo color-scheme picker in the footer. Off unless 'true'.
  SHOW_COLOR_SCHEME_SWITCHER: 'true',
  // 'tnsw' (default) lists consignments; 'agency' lists an agency's cases instead.
  APP_MODE: 'tnsw',
}
