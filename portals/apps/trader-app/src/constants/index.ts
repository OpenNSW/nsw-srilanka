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

// Application Routes. An agency portal serves the same screens for its cases, under /cases.
const CONSIGNMENTS_PATH = isAgencyMode ? '/cases' : '/consignments'

export const ROUTES = {
  HOME: '/',
  CONSIGNMENTS: CONSIGNMENTS_PATH,
  CONSIGNMENT_DETAIL: (id: string) => `${CONSIGNMENTS_PATH}/${id}`,
  TASK_FORM: (consignmentId: string, taskId: string) => `${CONSIGNMENTS_PATH}/${consignmentId}/tasks/${taskId}`,
  // Route patterns, for <Route path> and useMatch.
  CONSIGNMENT_DETAIL_PATTERN: `${CONSIGNMENTS_PATH}/:consignmentId`,
  TASK_FORM_PATTERN: `${CONSIGNMENTS_PATH}/:consignmentId/tasks/:taskId`,
} as const

// Pagination
export const PAGINATION = {
  DEFAULT_LIMIT: 20,
  DEFAULT_OFFSET: 0,
} as const

// Status Display Configurations
export const STATUS_COLORS = {
  IN_PROGRESS: 'orange',
  FINISHED: 'green',
  REQUIRES_REWORK: 'red',
  COMPLETED: 'green',
  READY: 'blue',
  LOCKED: 'gray',
} as const

export const TRADE_FLOW_COLORS = {
  IMPORT: 'blue',
  EXPORT: 'green',
} as const
