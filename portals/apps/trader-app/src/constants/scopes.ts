import { getEnv } from '@/runtimeConfig'

function configuredScope(name: string): string | undefined {
  return getEnv(name)?.trim()
}

// The scope each screen and action needs, set per deployment in the runtime
// config (config.js, or frontend.config in the Helm values). The portal shows
// a screen or action only to a user the IdP granted its scope. One whose key
// is unset is shown to nobody, so a deployment sets only the keys it uses.
export const ACCESS_SCOPES = {
  CONSIGNMENTS_VIEW: configuredScope('SCOPE_CONSIGNMENTS_VIEW'),
  CONSIGNMENTS_CREATE: configuredScope('SCOPE_CONSIGNMENTS_CREATE'),
  CONSIGNMENT_FLOWS: configuredScope('SCOPE_CONSIGNMENT_FLOWS'),
  TASKS_VIEW: configuredScope('SCOPE_TASKS_VIEW'),
  TASKS_ACTIONS: configuredScope('SCOPE_TASKS_ACTIONS'),
  ADMIN_VIEW: configuredScope('SCOPE_ADMIN_VIEW'),
  ADMIN_ACTIONS: configuredScope('SCOPE_ADMIN_ACTIONS'),
  PROFILE_VIEW: configuredScope('SCOPE_PROFILE_VIEW'),
} as const
