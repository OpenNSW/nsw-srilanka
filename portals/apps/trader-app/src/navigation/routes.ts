import { ROUTES } from '@/constants'
import { ACCESS_SCOPES } from '@/constants/scopes'

// A signed-in route: its path, and the scope its screen needs.
export interface AppRoute {
  path: string
  // Its screen, and any nav destination for it, are shown only to a user granted
  // this scope.
  scope: string
}

// The signed-in routes. The router restricts each screen, and the nav lists
// each destination, by the scope given here.
export const APP_ROUTES = {
  consignments: { path: ROUTES.CONSIGNMENTS, scope: ACCESS_SCOPES.CONSIGNMENTS_VIEW },
  consignmentDetail: { path: ROUTES.CONSIGNMENT_DETAIL_PATTERN, scope: ACCESS_SCOPES.CONSIGNMENTS_VIEW },
  task: { path: ROUTES.TASK_FORM_PATTERN, scope: ACCESS_SCOPES.TASKS_VIEW },
  adminConsignment: { path: ROUTES.ADMIN_CONSIGNMENT_PATTERN, scope: ACCESS_SCOPES.ADMIN_VIEW },
  adminConsignmentView: { path: ROUTES.ADMIN_CONSIGNMENT_VIEW_PATTERN, scope: ACCESS_SCOPES.ADMIN_VIEW },
} as const satisfies Record<string, AppRoute>
