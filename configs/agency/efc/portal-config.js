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
    'openid,profile,email,group,role,ou,consignment:read,task:read,task:write,storage:read,storage:write,profile:read',
  // The ID-token claim holding the officer's roles, which the user menu lists.
  IDP_ROLE_CLAIM_NAME: 'roles',
  IDP_ROLE_LABELS: JSON.stringify({
    'EFC Officer': {
      title: 'EFC Officer',
      description: 'Reviews and completes the tasks the EFC receives from the National Single Window.',
    },
  }),
  SHOW_AUTOFILL_BUTTON: 'false',
  APP_MODE: 'agency',
}
