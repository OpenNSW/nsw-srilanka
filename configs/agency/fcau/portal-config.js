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
    'openid,profile,email,group,role,ou,consignment:read,task:read,task:write,storage:read,storage:write,profile:read',
  // The ID-token claim holding the officer's roles, which the user menu lists.
  IDP_ROLE_CLAIM_NAME: 'roles',
  IDP_ROLE_LABELS: JSON.stringify({
    'FCAU Officer': {
      title: 'FCAU Officer',
      description: 'Reviews and completes the tasks the FCAU receives from the National Single Window.',
    },
  }),
  SHOW_AUTOFILL_BUTTON: 'false',
  APP_MODE: 'agency',
}
