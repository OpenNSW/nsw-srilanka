// Runtime config for the CDA officer portal: the trader-app in agency mode, served by
// the cda-portal service in compose.yml. See configs/agency/cda/README.md.
// The IdP side (resource server, CDA Officer role, this client) is idp/resources/government/cda.json.
window.__APP_CONFIG__ = {
  API_BASE_URL: 'http://localhost:8083',
  IDP_BASE_URL: 'https://localhost:8090',
  IDP_CLIENT_ID: 'OGA_PORTAL_APP_CDA',
  IDP_EXTRA_QUERY_PARAMS: 'resource=https://api.cda.nsw-agency.local',
  APP_URL: 'http://localhost:5176',
  IDP_SCOPES:
    'openid,profile,email,group,role,ou,nsw:consignment:read,nsw:task:read,nsw:task:write,nsw:storage:read,nsw:storage:write,nsw:profile:read',
  IDP_ROLE_CLAIM_NAME: 'roles',
  // Officers sign in as the app's "Trader" role; agency mode hides the role switcher
  // and the backend checks the real catalog roles (see docs/agency.md).
  IDP_TRADER_ROLE_NAME: 'CDA Officer',
  // TEMPORARY (#602 test): admits a user holding only the reviewer role, as the app's "CHA" role.
  IDP_CHA_ROLE_NAME: 'CDA Reviewer',
  IDP_NSW_ADMIN_ROLE_NAME: 'NSW Admin',
  SHOW_AUTOFILL_BUTTON: 'false',
  APP_MODE: 'agency',
}
