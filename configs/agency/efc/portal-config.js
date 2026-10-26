// Runtime config for the EFC officer portal: the trader-app in agency mode, served by
// the efc-portal service in compose.yml. See configs/agency/efc/README.md.
// The IdP side (resource server, EFC Officer role, this client) is idp/bootstrap/20-agency-efc.yaml.
window.__APP_CONFIG__ = {
  API_BASE_URL: 'http://localhost:8085',
  IDP_BASE_URL: 'http://localhost:8090',
  IDP_CLIENT_ID: 'OGA_PORTAL_APP_EFC',
  IDP_EXTRA_QUERY_PARAMS: 'resource=https://api.efc.nsw-agency.local',
  APP_URL: 'http://localhost:5178',
  IDP_SCOPES:
    'openid,profile,email,group,role,ou,nsw:consignment:read,nsw:task:read,nsw:task:write,nsw:storage:read,nsw:storage:write,nsw:profile:read',
  SHOW_AUTOFILL_BUTTON: 'false',
  APP_MODE: 'agency',
}
