// Runtime config for the CDA officer portal: the trader-app in agency mode, served by
// the cda-portal service in compose.yml. See configs/agency/cda/README.md.
// The IdP side (resource server, CDA roles, this client) is idp/bootstrap/20-agency-cda.yaml.
window.__APP_CONFIG__ = {
  API_BASE_URL: 'http://localhost:8083',
  IDP_BASE_URL: 'http://localhost:8090',
  IDP_CLIENT_ID: 'OGA_PORTAL_APP_CDA',
  IDP_EXTRA_QUERY_PARAMS: 'resource=https://api.cda.nsw-agency.local',
  APP_URL: 'http://localhost:5176',
  IDP_SCOPES:
    'openid,profile,email,group,role,ou,consignment:read,task:read,task:write,storage:read,storage:write,profile:read,consignment:adminread,consignment:adminwrite',
  // The ID-token claim holding the officer's roles, which the user menu reads.
  IDP_ROLE_CLAIM_NAME: 'roles',
  // The roles the user menu lists, with a title and description for each. Any other role
  // in the claim, such as one for another portal on the same IdP, is left out.
  IDP_ROLE_LABELS: JSON.stringify({
    'CDA Officer': {
      title: 'CDA Officer',
      description: 'Reviews and completes the tasks the CDA receives from the National Single Window.',
    },
    'CDA Application Reviewing Officer': {
      title: 'CDA Application Reviewing Officer',
      description: 'Verifies and reviews export coconut certificate applications.',
    },
    'CDA Application Reviewing Officer (In Lieu)': {
      title: 'CDA Application Reviewing Officer (In Lieu)',
      description: 'Stands in for the Application Reviewing Officer.',
    },
    'CDA Payment Confirming Officer': {
      title: 'CDA Payment Confirming Officer',
      description: 'Confirms that application fees reached the CDA account.',
    },
    'CDA Technical Support Officer': {
      title: 'CDA Technical Support Officer',
      description: 'Supports the review and certification of export coconut certificate applications.',
    },
    'CDA Quality Certificate Authorizing Officer': {
      title: 'CDA Quality Certificate Authorizing Officer',
      description: 'Authorizes and issues export coconut quality certificates.',
    },
    'CDA Quality Certificate Authorizing Officer (In Lieu)': {
      title: 'CDA Quality Certificate Authorizing Officer (In Lieu)',
      description: 'Stands in for the Quality Certificate Authorizing Officer.',
    },
    'CDA Processing Division Senior Officer': {
      title: 'CDA Processing Division Senior Officer',
      description: 'Oversees all processes of the Processing Division.',
    },
    'CDA Marketing Division Senior Officer': {
      title: 'CDA Marketing Division Senior Officer',
      description: 'Oversees export approvals and does the ASYCUDA green check for the Marketing Division.',
    },
    'CDA ASYCUDA Green Check Officer': {
      title: 'CDA ASYCUDA Green Check Officer',
      description: 'Does the ASYCUDA green check on export coconut certificates.',
    },
    'CDA ASYCUDA Green Check Officer (In Lieu)': {
      title: 'CDA ASYCUDA Green Check Officer (In Lieu)',
      description: 'Stands in for the ASYCUDA Green Check Officer.',
    },
    'CDA Admin': {
      title: 'CDA Admin',
      description: 'Views any case and its workflows, and resolves or force-parks their nodes.',
    },
  }),
  SHOW_AUTOFILL_BUTTON: 'false',
  APP_MODE: 'agency',
}
