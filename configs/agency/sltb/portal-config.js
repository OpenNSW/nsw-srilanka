// Runtime config for the SLTB officer portal: the trader-app in agency mode, served by
// the sltb-portal service in compose.yml. See configs/agency/sltb/README.md.
// The IdP side (resource server, SLTB Officer role, this client) is idp/bootstrap/20-agency-sltb.yaml.
window.__APP_CONFIG__ = {
  API_BASE_URL: 'http://localhost:8086',
  IDP_BASE_URL: 'http://localhost:8090',
  IDP_CLIENT_ID: 'OGA_PORTAL_APP_SLTB',
  IDP_EXTRA_QUERY_PARAMS: 'resource=https://api.sltb.nsw-agency.local',
  APP_URL: 'http://localhost:5179',
  IDP_SCOPES:
    'openid,profile,email,group,role,ou,nsw:consignment:read,nsw:task:read,nsw:task:write,nsw:storage:read,nsw:storage:write,nsw:profile:read',
  // The scope each screen and action this portal shows needs. A SCOPE_* key left unset
  // (here: creating consignments, their flows, the admin views) hides its
  // screen or action from everyone.
  SCOPE_CONSIGNMENTS_VIEW: 'nsw:consignment:read',
  SCOPE_TASKS_VIEW: 'nsw:task:read',
  SCOPE_TASKS_ACTIONS: 'nsw:task:write',
  SCOPE_PROFILE_VIEW: 'nsw:profile:read',
  SHOW_AUTOFILL_BUTTON: 'false',
  APP_MODE: 'agency',
}
