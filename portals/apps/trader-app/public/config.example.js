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
    'openid,profile,email,group,role,ou,consignment:read,consignment:write,consignment:flow:write,task:read,task:write,company:read,storage:read,storage:write,profile:read,consignment:adminread,consignment:adminwrite',
  // The ID-token claim holding the user's roles (an array of role names), which the user
  // menu lists. Needs the 'role' scope above. For an IdP that models roles as groups, set
  // the claim to 'groups'. Defaults to 'roles'.
  IDP_ROLE_CLAIM_NAME: 'roles',
  // The roles the user menu lists, keyed by the role name in the token, each with a title
  // and an optional description. The IdP may also put the user's roles for other
  // applications in the claim; the menu leaves out every role not named here.
  IDP_ROLE_LABELS: JSON.stringify({
    Trader: { title: 'Trader', description: 'Creates and manages consignments for your company.' },
    CHA: { title: 'Customs House Agent', description: 'Acts for traders on their consignments.' },
    'NSW Admin': { title: 'NSW Administrator', description: 'Administers consignments across all traders.' },
  }),
  SHOW_AUTOFILL_BUTTON: 'true',
  // 'tnsw' (default) lists consignments; 'agency' lists an agency's cases instead.
  APP_MODE: 'tnsw',
}
