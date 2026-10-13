import type { ConsignmentState, WorkflowNode } from '@/features/consignment/types'

const SLTB_BLEND_SHEET_REVIEW_TITLE = '[SLTB] Submit & Review Blend Sheet'

/**
 * True when this completed blend-sheet review ended the consignment by officer
 * rejection: FINISHED, and no later SLTB step exists (approval creates Pay Levy+).
 */
export function isRejectedSltbBlendSheetStep(
  step: WorkflowNode,
  allSteps: WorkflowNode[],
  consignmentState?: ConsignmentState,
): boolean {
  if (consignmentState !== 'FINISHED' || step.state !== 'COMPLETED') return false
  if ((step.workflowNodeTemplate.name ?? '') !== SLTB_BLEND_SHEET_REVIEW_TITLE) return false
  // Titles are "[SLTB] …" or "[SLTB 3A] …" — match the common "[SLTB" prefix.
  return !allSteps.some((other) => other.id !== step.id && (other.workflowNodeTemplate.name ?? '').startsWith('[SLTB'))
}

export function isRejectedSltbConsignment(steps: WorkflowNode[], consignmentState?: ConsignmentState): boolean {
  return steps.some((step) => isRejectedSltbBlendSheetStep(step, steps, consignmentState))
}
