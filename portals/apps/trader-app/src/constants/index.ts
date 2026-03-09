/**
 * Application-wide constants
 */

import { getEnv } from '../runtimeConfig'

// TODO: Replace with actual auth context
export const DEFAULT_TRADER_ID = 'trader-123'

// API Configuration
export const API_CONFIG = {
  BASE_URL: getEnv('VITE_API_BASE_URL', 'http://localhost:8080/api/v1')!,
  TIMEOUT: 30000, // 30 seconds
} as const

// Application Routes
export const ROUTES = {
  HOME: '/',
  CONSIGNMENTS: '/consignments',
  CONSIGNMENT_DETAIL: (id: string) => `/consignments/${id}`,
  TASK_FORM: (consignmentId: string, taskId: string) =>
    `/consignments/${consignmentId}/tasks/${taskId}`,
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
