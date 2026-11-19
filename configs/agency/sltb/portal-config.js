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
    'openid,profile,email,group,role,ou,consignment:read,task:read,task:write,storage:read,storage:write,profile:read,consignment:adminread,consignment:adminwrite',
  // The ID-token claim holding the officer's roles, which the user menu reads.
  IDP_ROLE_CLAIM_NAME: 'roles',
  // The roles the user menu lists, with a title and description for each. Any other role
  // in the claim, such as one for another portal on the same IdP, is left out.
  IDP_ROLE_LABELS: JSON.stringify({
    'SLTB Officer': {
      title: 'SLTB Officer',
      description: 'Reviews and completes the tasks the SLTB receives from the National Single Window.',
    },
    'SLTB Admin': {
      title: 'SLTB Admin',
      description: 'Views any case and its workflows, and resolves or force-parks their nodes.',
    },
  }),
  SHOW_AUTOFILL_BUTTON: 'false',
  APP_MODE: 'agency',
}
