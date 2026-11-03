import { ROUTES } from '@/constants'
import type { Ability } from '@/hooks/useCan'

// A signed-in route: its path, and the ability its screen needs.
export interface AppRoute {
  path: string
  // Its screen, and any nav destination for it, are shown only to a user who
  // may do this.
  ability: Ability
}

// The signed-in routes. The router restricts each screen, and the nav lists
// each destination, by the ability given here.
export const APP_ROUTES = {
  consignments: { path: ROUTES.CONSIGNMENTS, ability: 'consignment.view' },
  consignmentDetail: { path: ROUTES.CONSIGNMENT_DETAIL_PATTERN, ability: 'consignment.view' },
  task: { path: ROUTES.TASK_FORM_PATTERN, ability: 'task.view' },
  adminConsignment: { path: ROUTES.ADMIN_CONSIGNMENT_PATTERN, ability: 'admin.view' },
  adminConsignmentView: { path: ROUTES.ADMIN_CONSIGNMENT_VIEW_PATTERN, ability: 'admin.view' },
} as const satisfies Record<string, AppRoute>
