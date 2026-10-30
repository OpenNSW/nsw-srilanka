// The runtime config every test file starts with (see setup.ts), shaped like a
// deployment's config.js.
export const TEST_APP_CONFIG = {
  API_BASE_URL: 'http://api.test',
  SCOPE_CONSIGNMENTS_VIEW: 'nsw:consignment:read',
  SCOPE_CONSIGNMENTS_CREATE: 'nsw:consignment:write',
  SCOPE_CONSIGNMENT_FLOWS: 'nsw:consignment:flow',
  SCOPE_TASKS_VIEW: 'nsw:task:read',
  SCOPE_TASKS_ACTIONS: 'nsw:task:write',
  SCOPE_ADMIN_VIEW: 'nsw:consignment:adminread',
  SCOPE_ADMIN_ACTIONS: 'nsw:consignment:adminwrite',
  SCOPE_PROFILE_VIEW: 'nsw:profile:read',
}
