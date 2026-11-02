// Local dev runtime config. Copy this file to config.js once per clone
// (config.js is gitignored) — this is the single source of truth for local
// dev, used both by `vite dev` (served as-is from public/) and by
// `docker compose` (bind-mounted into the container — see compose.yml).
// A real deployment supplies its own values via Helm (see
// deployments/helm/values-example.yaml). IDP_SCOPES must include the
// API scopes the backend checks, or it rejects the resulting token.
window.__APP_CONFIG__ = {
  API_BASE_URL: 'http://localhost:8080',
  IDP_BASE_URL: 'http://localhost:8090',
  IDP_CLIENT_ID: 'TRADER_PORTAL_APP',
  IDP_EXTRA_QUERY_PARAMS: 'resource=https://api.nsw-srilanka.local',
  APP_URL: 'http://localhost:5173',
  IDP_SCOPES:
    'openid,profile,email,group,role,ou,consignment:read,consignment:write,task:read,task:write,hscode:read,company:read,cha:read,storage:read,storage:write,profile:read,consignment:adminread,consignment:adminwrite',
  // ID-token claim holding the roles (an array), and the role names in it. For an
  // IdP that models roles as groups, set the claim to 'groups' and use the group names.
  SHOW_AUTOFILL_BUTTON: 'true',
  // Shows the demo color-scheme picker in the footer. Off unless 'true'.
  SHOW_COLOR_SCHEME_SWITCHER: 'true',
  // 'tnsw' (default) lists consignments; 'agency' lists an agency's cases instead.
  APP_MODE: 'tnsw',
}
