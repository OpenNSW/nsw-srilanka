import type { TaskClaim } from '@/features/zone/types'

// showClaimBanner reports whether the task's claim is worth showing: where the
// deployment requires a claim to act (agency mode), or someone holds it. A
// deployment that requires none and where nobody claims sees no banner at all.
export function showClaimBanner(claim: TaskClaim | undefined): claim is TaskClaim {
  return claim !== undefined && (claim.required_to_submit || claim.holder !== undefined)
}

// canActOnTask reports whether the claim lets the caller act on the task: always
// where the deployment requires no claim, otherwise only when the caller holds it.
// The backend refuses the submission otherwise (403).
export function canActOnTask(claim: TaskClaim | undefined): boolean {
  return !claim?.required_to_submit || claim.mine
}
