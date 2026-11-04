// Runtime config for the CDA officer portal: the trader-app in agency mode, served by
// the cda-portal service in compose.yml. See configs/agency/cda/README.md.
// The IdP side (resource server, CDA Officer role, this client) is idp/bootstrap/20-agency-cda.yaml.
window.__APP_CONFIG__ = {
  API_BASE_URL: 'http://localhost:8083',
  IDP_BASE_URL: 'http://localhost:8090',
  IDP_CLIENT_ID: 'OGA_PORTAL_APP_CDA',
  IDP_EXTRA_QUERY_PARAMS: 'resource=https://api.cda.nsw-agency.local',
  APP_URL: 'http://localhost:5176',
  IDP_SCOPES:
    'openid,profile,email,group,role,ou,consignment:read,task:read,task:write,storage:read,storage:write,profile:read,consignment:adminread,consignment:adminwrite',
  // The ID-token claim holding the officer's roles, which the user menu lists.
  IDP_ROLE_CLAIM_NAME: 'roles',
  IDP_ROLE_LABELS: JSON.stringify({
    'CDA Officer': {
      title: 'CDA Officer',
      description: 'Reviews and completes the tasks the CDA receives from the National Single Window.',
    },
    'CDA Admin': {
      title: 'CDA Admin',
      description: 'Views any case and its workflows, and resolves or force-parks their nodes.',
    },
  }),
  SHOW_AUTOFILL_BUTTON: 'false',
  APP_MODE: 'agency',
}
