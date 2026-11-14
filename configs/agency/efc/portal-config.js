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
    'openid,profile,email,group,role,ou,consignment:read,task:read,task:write,storage:read,storage:write,profile:read,consignment:adminread,consignment:adminwrite',
  // The ID-token claim holding the officer's roles, which the user menu reads.
  IDP_ROLE_CLAIM_NAME: 'roles',
  // The roles the user menu lists, with a title and description for each. Any other role
  // in the claim, such as one for another portal on the same IdP, is left out.
  IDP_ROLE_LABELS: JSON.stringify({
    'EFC Officer': {
      title: 'EFC Officer',
      description: 'Reviews and completes the tasks the EFC receives from the National Single Window.',
    },
    'EFC Admin': {
      title: 'EFC Admin',
      description: 'Views any case and its workflows, and resolves or force-parks their nodes.',
    },
  }),
  SHOW_AUTOFILL_BUTTON: 'false',
  APP_MODE: 'agency',
}
