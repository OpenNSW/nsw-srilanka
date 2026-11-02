// Local dev runtime config. Copy this file to config.js once per clone
// (config.js is gitignored) — this is the single source of truth for local
// dev, used both by `vite dev` (served as-is from public/) and by
// `docker compose` (bind-mounted into the container — see compose.yml).
// A real deployment supplies its own values via Helm (see
// deployments/helm/values-example.yaml). IDP_SCOPES must include the
// API scopes: the backend checks them on every call, and the portal
// shows a screen or action only to a user the IdP granted its scope.
window.__APP_CONFIG__ = {
  API_BASE_URL: 'http://localhost:8080',
  IDP_BASE_URL: 'http://localhost:8090',
  IDP_CLIENT_ID: 'TRADER_PORTAL_APP',
  IDP_EXTRA_QUERY_PARAMS: 'resource=https://api.nsw-srilanka.local',
  APP_URL: 'http://localhost:5173',
  IDP_SCOPES:
    'openid,profile,email,group,role,ou,consignment:read,consignment:write,consignment:flow:write,task:read,task:write,hscode:read,company:read,cha:read,storage:read,storage:write,profile:read,consignment:adminread,consignment:adminwrite',
  // The scope each screen and action needs: the portal shows it only to a user
  // the IdP granted that scope. A SCOPE_* key left unset hides its screen or
  // action from everyone.
  SCOPE_CONSIGNMENTS_VIEW: 'consignment:read',
  SCOPE_CONSIGNMENTS_CREATE: 'consignment:write',
  SCOPE_CONSIGNMENT_FLOWS: 'consignment:flow:write',
  SCOPE_TASKS_VIEW: 'task:read',
  SCOPE_TASKS_ACTIONS: 'task:write',
  SCOPE_ADMIN_VIEW: 'consignment:adminread',
  SCOPE_ADMIN_ACTIONS: 'consignment:adminwrite',
  SCOPE_PROFILE_VIEW: 'profile:read',
  SHOW_AUTOFILL_BUTTON: 'true',
  // Shows the demo color-scheme picker in the footer. Off unless 'true'.
  SHOW_COLOR_SCHEME_SWITCHER: 'true',
  // 'tnsw' (default) lists consignments; 'agency' lists an agency's cases instead.
  APP_MODE: 'tnsw',
}
