import { createRoleMapper } from '@opennsw/auth'
import { getEnv } from '@/runtimeConfig'
import type { Role } from '@/services/RoleContext'

// `roles` is the claim ThunderID releases for the `role` scope, and the one the backend reads.
// Key order sets the default active role when a user has several.
export const mapClaimsToRoles = createRoleMapper<Role>({
  claimName: getEnv('IDP_ROLE_CLAIM_NAME', 'roles'),
  roles: {
    trader: getEnv('IDP_TRADER_ROLE_NAME', 'Trader'),
    nswAdmin: getEnv('IDP_NSW_ADMIN_ROLE_NAME', 'NSW Admin'),
    cha: getEnv('IDP_CHA_ROLE_NAME', 'CHA'),
  },
})
