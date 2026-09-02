import type { ConsignmentState, WorkflowNodeState } from './types'
import i18n from '@/i18n'

/**
 * Get the appropriate color for a consignment state badge.
 *
 * Returns Radix UI semantic color names (consumed by Radix <Badge color={...}>).
 * Radix is aligned to the brand tokens in main.tsx (<Theme>), so these map to:
 *   orange → warning, green → success, red → error, gray → secondary.
 * See the token block in index.css.
 */
export function getStateColor(state: ConsignmentState): 'gray' | 'orange' | 'green' | 'red' {
  switch (state) {
    case 'INITIALIZED':
    case 'IN_PROGRESS':
      return 'orange'
    case 'FINISHED':
      return 'green'
    case 'FAILED':
      return 'red'
    default:
      return 'gray'
  }
}

/**
 * Format a consignment state for display
 * Converts underscore-separated uppercase to title case with spaces
 * Example: IN_PROGRESS -> In Progress
 */
export function formatState(state: ConsignmentState): string {
  return state.replace('_', ' ').replace(/\b\w/g, (c) => c.toUpperCase())
}

/**
 * Render-config section keys that agency artifacts use to show officer feedback
 * to the trader (visibleWhen.requireDataKey). Exact section names only.
 */
const FEEDBACK_VIEW_KEYS = new Set(['assessment_outcome', 'feedback'])

function viewHasFeedbackSurface(view: object | undefined): boolean {
  if (!view) {
    return false
  }
  return Object.keys(view).some((key) => FEEDBACK_VIEW_KEYS.has(key))
}

/**
 * Overlay Pending Feedback when GET /tasks/{id} is user-pending and the
 * projected view is showing an officer-feedback section. Otherwise keep the
 * consignment API state (typically IN_PROGRESS).
 */
export function deriveTraderFacingState(
  apiState: WorkflowNodeState,
  zone?: { state: string; view?: object } | null,
): WorkflowNodeState {
  if (!zone) {
    return apiState
  }
  const userPending = zone.state === 'PENDING_USER' || zone.state === 'IN_PROGRESS' || zone.state === 'READY'
  if (userPending && viewHasFeedbackSurface(zone.view)) {
    return 'PENDING_FEEDBACK'
  }
  return apiState
}

/**
 * Format a date string for display using the active locale.
 * Example: 2026-01-27T10:30:00Z -> Jan 27, 2026
 */
export function formatDate(dateString: string): string {
  return new Date(dateString).toLocaleDateString(i18n.resolvedLanguage || undefined, {
    year: 'numeric',
    month: 'short',
    day: 'numeric',
  })
}

/**
 * Format a date string with time for display using the active locale.
 * Produces locale-appropriate output using the dateTimeAt translation template.
 * Example: 2026-01-27T10:30:00Z -> January 27, 2026 at 10:30 AM
 */
export function formatDateTime(dateString: string): string {
  const date = new Date(dateString)
  if (isNaN(date.getTime())) {
    return '-'
  }
  const lang = i18n.resolvedLanguage || undefined
  const datePart = date.toLocaleDateString(lang, {
    year: 'numeric',
    month: 'long',
    day: 'numeric',
  })
  const timePart = date.toLocaleTimeString(lang, {
    hour: '2-digit',
    minute: '2-digit',
  })
  return i18n.t('common.dateTimeAt', { date: datePart, time: timePart })
}
