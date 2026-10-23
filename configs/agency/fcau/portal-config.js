// Runtime config for the FCAU officer portal: the trader-app in agency mode, served by
// the fcau-portal service in compose.yml. See configs/agency/fcau/README.md.
// The IdP side (resource server, FCAU Officer role, this client) is idp/bootstrap/20-agency-fcau.yaml.
window.__APP_CONFIG__ = {
  API_BASE_URL: 'http://localhost:8082',
  IDP_BASE_URL: 'http://localhost:8090',
  IDP_CLIENT_ID: 'OGA_PORTAL_APP_FCAU',
  IDP_EXTRA_QUERY_PARAMS: 'resource=https://api.fcau.nsw-agency.local',
  APP_URL: 'http://localhost:5175',
  IDP_SCOPES:
    'openid,profile,email,group,role,ou,nsw:consignment:read,nsw:task:read,nsw:task:write,nsw:storage:read,nsw:storage:write,nsw:profile:read',
  SHOW_AUTOFILL_BUTTON: 'false',
  APP_MODE: 'agency',
}
