import type { WorkflowNodeState } from '@/features/consignment/types'

export const WORKFLOW_STATUS_I18N_KEYS: Record<
  WorkflowNodeState,
  'completed' | 'ready' | 'inProgress' | 'locked' | 'failed' | 'awaitingFeedback'
> = {
  COMPLETED: 'completed',
  READY: 'ready',
  IN_PROGRESS: 'inProgress',
  QUEUED_EXTERNALLY: 'awaitingFeedback',
  LOCKED: 'locked',
  FAILED: 'failed',
}

export function workflowStatusI18nKey(state: string): string | undefined {
  const key = WORKFLOW_STATUS_I18N_KEYS[state as WorkflowNodeState]
  return key ? `workflow.status.${key}` : undefined
}
