import { getIdpRoleConfig, type IdpRoleConfig } from '@/runtimeConfig'
import type { Role } from '@/services/RoleContext'

const TRADER_ROLE: Role = 'trader'
const CHA_ROLE: Role = 'cha'
const NSW_ADMIN_ROLE: Role = 'nswAdmin'

function toRoleNameSet(roleClaim: unknown): Set<string> {
  if (!Array.isArray(roleClaim)) {
    return new Set()
  }

  return new Set(roleClaim.filter((value): value is string => typeof value === 'string'))
}

export function mapRoleNamesToRoles(roleClaim: unknown, config: IdpRoleConfig): Role[] {
  const roleNames = toRoleNameSet(roleClaim)
  const roles: Role[] = []

  if (roleNames.has(config.traderRoleName)) {
    roles.push(TRADER_ROLE)
  }

  if (roleNames.has(config.nswAdminRoleName)) {
    roles.push(NSW_ADMIN_ROLE)
  }

  if (roleNames.has(config.chaRoleName)) {
    roles.push(CHA_ROLE)
  }

  return roles
}

export function mapClaimsToRoles(
  claims: Record<string, unknown> | null | undefined,
  config: IdpRoleConfig = getIdpRoleConfig(),
): Role[] {
  return mapRoleNamesToRoles(claims?.[config.roleClaimName], config)
}
