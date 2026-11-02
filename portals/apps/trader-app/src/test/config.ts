// The runtime config every test file starts with (see setup.ts), shaped like a
// deployment's config.js.
export const TEST_APP_CONFIG = {
  API_BASE_URL: 'http://api.test',
  SCOPE_CONSIGNMENTS_VIEW: 'consignment:read',
  SCOPE_CONSIGNMENTS_CREATE: 'consignment:write',
  SCOPE_CONSIGNMENT_FLOWS: 'consignment:flow:write',
  SCOPE_TASKS_VIEW: 'task:read',
  SCOPE_TASKS_ACTIONS: 'task:write',
  SCOPE_ADMIN_VIEW: 'consignment:adminread',
  SCOPE_ADMIN_ACTIONS: 'consignment:adminwrite',
  SCOPE_PROFILE_VIEW: 'profile:read',
}
