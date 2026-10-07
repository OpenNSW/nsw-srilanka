// Runtime config for the Customs officer portal: the trader-app in agency mode, served by
// the customs-portal service in compose.yml. See configs/agency/customs/README.md.
// The IdP side (resource server, Customs Officer role, this client) is idp/resources/government/customs.json.
window.__APP_CONFIG__ = {
  API_BASE_URL: 'http://localhost:8085',
  IDP_BASE_URL: 'https://localhost:8090',
  IDP_CLIENT_ID: 'OGA_PORTAL_APP_CUSTOMS',
  IDP_EXTRA_QUERY_PARAMS: 'resource=https://api.customs.nsw-agency.local',
  APP_URL: 'http://localhost:5178',
  IDP_SCOPES:
    'openid,profile,email,group,role,ou,nsw:consignment:read,nsw:task:read,nsw:task:write,nsw:storage:read,nsw:storage:write,nsw:storage:delete,nsw:profile:read',
  IDP_ROLE_CLAIM_NAME: 'roles',
  // Officers sign in as the app's "Trader" role; agency mode hides the role switcher
  // and the backend checks the real officer role (see docs/agency.md).
  IDP_TRADER_ROLE_NAME: 'Customs Officer',
  IDP_CHA_ROLE_NAME: 'CHA',
  IDP_NSW_ADMIN_ROLE_NAME: 'NSW Admin',
  SHOW_AUTOFILL_BUTTON: 'false',
  APP_MODE: 'agency',
}
