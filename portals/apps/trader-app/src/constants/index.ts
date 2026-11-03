/**
 * Application-wide constants
 */

import { getRequiredEnv } from '@/runtimeConfig'
import { isAgencyMode } from '@/features/case/mode'

export const API_BASE_URL = getRequiredEnv('API_BASE_URL')

// API Configuration
export const API_CONFIG = {
  BASE_URL: API_BASE_URL,
  TIMEOUT: 30000, // 30 seconds
} as const

// Application Routes. An agency portal serves the same screens for its cases, under /cases —
// including the admin debugger, under /admin/cases (ADMIN_CONSIGNMENT_VIEW has no case
// equivalent: it is never routed to in agency mode, see App.tsx).
const CONSIGNMENTS_PATH = isAgencyMode ? '/cases' : '/consignments'
const ADMIN_CONSIGNMENTS_PATH = isAgencyMode ? '/admin/cases' : '/admin/consignments'

export const ROUTES = {
  HOME: '/',
  CONSIGNMENTS: CONSIGNMENTS_PATH,
  CONSIGNMENT_DETAIL: (id: string) => `${CONSIGNMENTS_PATH}/${id}`,
  TASK_FORM: (consignmentId: string, taskId: string) => `${CONSIGNMENTS_PATH}/${consignmentId}/tasks/${taskId}`,
  ADMIN_CONSIGNMENT: (id: string) => `${ADMIN_CONSIGNMENTS_PATH}/${id}`,
  ADMIN_CONSIGNMENT_VIEW: (id: string) => `${ADMIN_CONSIGNMENTS_PATH}/${id}/view`,
  // Route patterns, for <Route path> and useMatch.
  CONSIGNMENT_DETAIL_PATTERN: `${CONSIGNMENTS_PATH}/:consignmentId`,
  TASK_FORM_PATTERN: `${CONSIGNMENTS_PATH}/:consignmentId/tasks/:taskId`,
  ADMIN_CONSIGNMENT_PATTERN: `${ADMIN_CONSIGNMENTS_PATH}/:consignmentId`,
  ADMIN_CONSIGNMENT_VIEW_PATTERN: `${ADMIN_CONSIGNMENTS_PATH}/:consignmentId/view`,
} as const

// Pagination
export const PAGINATION = {
  DEFAULT_LIMIT: 20,
  DEFAULT_OFFSET: 0,
} as const
