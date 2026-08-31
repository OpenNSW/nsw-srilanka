import type { ConsignmentState } from './types'
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

export type NodeStatusColor = 'green' | 'blue' | 'orange' | 'gray' | 'red'

export type NodeStatusLabelKey =
  'completed' | 'ready' | 'inProgress' | 'locked' | 'failed' | 'pendingFeedback' | 'awaitingReview' | 'pendingPayment'

export type NodeStatusAppearance = {
  color: NodeStatusColor
  labelKey?: NodeStatusLabelKey
  fallbackLabel: string
}

/**
 * Map a workflow-node state (orchestrator or derived) to trader-facing badge
 * colour and copy. Unknown states (status_override values from artifacts) are
 * title-cased so agency-specific parked states still read clearly.
 */
export function getNodeStatusAppearance(state: string): NodeStatusAppearance {
  switch (state) {
    case 'COMPLETED':
      return { color: 'green', labelKey: 'completed', fallbackLabel: 'Completed' }
    case 'READY':
      return { color: 'blue', labelKey: 'ready', fallbackLabel: 'Ready' }
    case 'LOCKED':
      return { color: 'gray', labelKey: 'locked', fallbackLabel: 'Locked' }
    case 'FAILED':
      return { color: 'red', labelKey: 'failed', fallbackLabel: 'Failed' }
    case 'PENDING_FEEDBACK':
      return { color: 'orange', labelKey: 'pendingFeedback', fallbackLabel: 'Pending Feedback' }
    case 'QUEUED_EXTERNALLY':
      return { color: 'blue', labelKey: 'awaitingReview', fallbackLabel: 'Awaiting Review' }
    case 'PENDING_PAYMENT':
      return { color: 'orange', labelKey: 'pendingPayment', fallbackLabel: 'Pending Payment' }
    case 'PENDING_USER':
    case 'IN_PROGRESS':
      return { color: 'orange', labelKey: 'inProgress', fallbackLabel: 'In Progress' }
    default:
      return { color: 'orange', fallbackLabel: humanizeNodeState(state) }
  }
}

export function isFinishedNodeState(state: string): boolean {
  return state === 'COMPLETED' || state === 'FAILED'
}

export function isLockedNodeState(state: string): boolean {
  return state === 'LOCKED'
}

function humanizeNodeState(state: string): string {
  return state
    .toLowerCase()
    .split('_')
    .filter(Boolean)
    .map((word) => word.charAt(0).toUpperCase() + word.slice(1))
    .join(' ')
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
