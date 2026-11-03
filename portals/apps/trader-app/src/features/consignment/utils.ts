import type { StatusTone } from '@portals/ui'
import type { ConsignmentState } from './types'
import i18n from '@/i18n'
import { humanizeStatus } from '@/utils/formatStatus'

/**
 * The StatusBadge tone for a consignment state. In progress is info, as an
 * in-progress task is (workflowStatusTone), so the same label never shows in
 * two colors on one page.
 */
export function getStateTone(state: ConsignmentState): StatusTone {
  switch (state) {
    case 'IN_PROGRESS':
      return 'info'
    case 'FINISHED':
      return 'success'
    default:
      return 'neutral'
  }
}

/**
 * Format a consignment state for display
 * Converts underscore-separated uppercase to title case with spaces
 * Example: IN_PROGRESS -> In Progress
 */
export function formatState(state: ConsignmentState): string {
  switch (state) {
    case 'IN_PROGRESS':
      return i18n.t('consignments.state.inProgress')
    case 'FINISHED':
      return i18n.t('consignments.state.finished')
    default:
      return humanizeStatus(state)
  }
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
