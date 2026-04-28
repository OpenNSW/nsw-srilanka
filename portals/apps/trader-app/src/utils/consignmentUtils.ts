import type { ConsignmentState } from '../services/types/consignment'

/**
 * Get the appropriate color for a consignment state badge
 */
export function getStateColor(
  state: ConsignmentState
): 'gray' | 'orange' | 'green' | 'red' {
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
 * Format a date string for display
 * Example: 2026-01-27T10:30:00Z -> Jan 27, 2026
 */
export function formatDate(dateString: string): string {
  return new Date(dateString).toLocaleDateString('en-US', {
    year: 'numeric',
    month: 'short',
    day: 'numeric',
  })
}

/**
 * Format a date string with time for display
 * Example: 2026-01-27T10:30:00Z -> January 27, 2026 at 10:30 AM
 */
export function formatDateTime(dateString: string): string {
  const date = new Date(dateString)
  if (isNaN(date.getTime())) {
    return '-'
  }
  return date.toLocaleDateString('en-US', {
    year: 'numeric',
    month: 'long',
    day: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
  })
}
