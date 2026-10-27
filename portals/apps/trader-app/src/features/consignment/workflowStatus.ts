import type { WorkflowNodeState } from '@/features/consignment/types'

export const WORKFLOW_STATUS_I18N_KEYS: Record<
  WorkflowNodeState,
  'completed' | 'ready' | 'inProgress' | 'locked' | 'failed' | 'awaitingFeedback' | 'cancelled' | 'onHold'
> = {
  COMPLETED: 'completed',
  READY: 'ready',
  IN_PROGRESS: 'inProgress',
  QUEUED_EXTERNALLY: 'awaitingFeedback',
  LOCKED: 'locked',
  FAILED: 'failed',
  CANCELLED: 'cancelled',
  ON_HOLD: 'onHold',
}

type WorkflowStatusI18nKey = `workflow.status.${(typeof WORKFLOW_STATUS_I18N_KEYS)[WorkflowNodeState]}`

// The task page shows the engine's own state rather than the node state above.
// A task waiting for input reads "In Progress" there too, not "Pending User",
// which sounds like it is waiting on whoever is looking at it.
const TASK_STATE_ALIASES: Partial<Record<string, WorkflowNodeState>> = {
  PENDING_USER: 'IN_PROGRESS',
}

export function workflowStatusI18nKey(state: string): WorkflowStatusI18nKey | undefined {
  const key = WORKFLOW_STATUS_I18N_KEYS[(TASK_STATE_ALIASES[state] ?? state) as WorkflowNodeState]
  return key ? `workflow.status.${key}` : undefined
}
