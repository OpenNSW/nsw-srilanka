import { getIdpRoleGroupConfig, type IdpRoleGroupConfig } from '@/runtimeConfig'
import type { Role } from '@/services/RoleContext'

interface ClaimsWithGroups {
  groups?: unknown
}

const TRADER_ROLE: Role = 'trader'
const CHA_ROLE: Role = 'cha'

function toGroupSet(groupsClaim: unknown): Set<string> {
  if (!Array.isArray(groupsClaim)) {
    return new Set()
  }

  return new Set(groupsClaim.filter((value): value is string => typeof value === 'string'))
}

export function mapGroupsToRoles(groupsClaim: unknown, config: IdpRoleGroupConfig): Role[] {
  const groups = toGroupSet(groupsClaim)
  const roles: Role[] = []

  if (groups.has(config.traderGroupName)) {
    roles.push(TRADER_ROLE)
  }

  if (groups.has(config.chaGroupName)) {
    roles.push(CHA_ROLE)
  }

  return roles
}

export function mapClaimsToRoles(
  claims: ClaimsWithGroups | null | undefined,
  config: IdpRoleGroupConfig = getIdpRoleGroupConfig(),
): Role[] {
  return mapGroupsToRoles(claims?.groups, config)
}
