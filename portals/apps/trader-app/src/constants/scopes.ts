// The scope each screen and action needs: the NSW API scope its calls require
// (internal/scopes/scopes.go). The portal shows a screen or action only to a
// user the IdP granted its scope.
export const ACCESS_SCOPES = {
  CONSIGNMENTS_VIEW: 'consignment:read',
  CONSIGNMENTS_CREATE: 'consignment:write',
  CONSIGNMENT_FLOWS: 'consignment:flow:write',
  TASKS_VIEW: 'task:read',
  TASKS_ACTIONS: 'task:write',
  ADMIN_VIEW: 'consignment:adminread',
  ADMIN_ACTIONS: 'consignment:adminwrite',
  PROFILE_VIEW: 'profile:read',
} as const
