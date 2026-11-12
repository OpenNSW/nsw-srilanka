// The NSW API scopes the portal's screens and actions need, as the backend
// names them (internal/scopes/scopes.go).
export const SCOPES = {
  CONSIGNMENT_READ: 'consignment:read',
  CONSIGNMENT_WRITE: 'consignment:write',
  TASK_READ: 'task:read',
  TASK_WRITE: 'task:write',
  CONSIGNMENT_ADMIN_READ: 'consignment:adminread',
  CONSIGNMENT_ADMIN_WRITE: 'consignment:adminwrite',
  PROFILE_READ: 'profile:read',
} as const
