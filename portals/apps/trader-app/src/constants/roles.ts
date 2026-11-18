import { z } from 'zod'
import { getEnv } from '@/runtimeConfig'

export interface RoleLabel {
  title: string
  description?: string
}

const RoleLabelsSchema = z.record(
  z.string(),
  z.object({
    title: z.string().trim().min(1),
    description: z.string().optional(),
  }),
)

// The role labels from the runtime config's IDP_ROLE_LABELS: JSON that maps a
// role name, as it appears in the token, to its title and optional description.
// The user menu lists only the roles named here. Labels only decorate the menu,
// so a malformed value is reported and ignored, and the menu then lists no roles.
export function parseRoleLabels(raw: string | undefined): Record<string, RoleLabel> {
  if (!raw) {
    return {}
  }

  try {
    return RoleLabelsSchema.parse(JSON.parse(raw))
  } catch (error) {
    console.error(
      'Ignoring IDP_ROLE_LABELS: expected JSON like {"<role>": {"title": "...", "description": "..."}}',
      error,
    )
    return {}
  }
}

// The ID-token claim that holds the user's roles, and which of them the user
// menu lists and how it labels each one. Both are set per deployment in the runtime config (config.js, or
// frontend.config in the Helm values).
export const ROLE_CLAIM_NAME = getEnv('IDP_ROLE_CLAIM_NAME', 'roles').trim()
export const ROLE_LABELS = parseRoleLabels(getEnv('IDP_ROLE_LABELS'))
