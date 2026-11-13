// Runtime config for the NPQS officer portal: the trader-app in agency mode, served by
// the npqs-portal service in compose.yml. See configs/agency/npqs/README.md.
// The IdP side (resource server, NPQS roles, this client) is idp/bootstrap/20-agency-npqs.yaml.
window.__APP_CONFIG__ = {
  API_BASE_URL: 'http://localhost:8081',
  IDP_BASE_URL: 'http://localhost:8090',
  IDP_CLIENT_ID: 'OGA_PORTAL_APP_NPQS',
  IDP_EXTRA_QUERY_PARAMS: 'resource=https://api.npqs.nsw-agency.local',
  APP_URL: 'http://localhost:5174',
  IDP_SCOPES:
    'openid,profile,email,group,role,ou,consignment:read,task:read,task:write,storage:read,storage:write,profile:read,consignment:adminread,consignment:adminwrite',
  // The ID-token claim holding the officer's roles, which the user menu lists.
  IDP_ROLE_CLAIM_NAME: 'roles',
  IDP_ROLE_LABELS: JSON.stringify({
    'NPQS Officer': {
      title: 'NPQS Officer',
      description: 'Reviews and completes the tasks the NPQS receives from the National Single Window.',
    },
    'NPPO Assistant': {
      title: 'NPPO Assistant',
      description: 'Assists NPPO officers with the tasks the NPQS receives from the National Single Window.',
    },
    'NPPO Inspector': {
      title: 'NPPO Inspector',
      description: 'Inspects consignments for the tasks the NPQS receives from the National Single Window.',
    },
    'NPPO Officer': {
      title: 'NPPO Officer',
      description: 'Reviews and completes the tasks the NPQS receives from the National Single Window.',
    },
    'NPQS Admin': {
      title: 'NPQS Admin',
      description: 'Views any case and its workflows, and resolves or force-parks their nodes.',
    },
  }),
  SHOW_AUTOFILL_BUTTON: 'false',
  APP_MODE: 'agency',
}
