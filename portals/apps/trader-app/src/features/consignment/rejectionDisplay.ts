import type { ConsignmentState, WorkflowNode } from '@/features/consignment/types'

const SLTB_BLEND_SHEET_REVIEW_TITLE = '[SLTB] Submit & Review Blend Sheet'

/**
 * True when this completed blend-sheet review ended the SLTB branch by officer
 * rejection: no later SLTB step exists (approval would create Pay Levy+).
 *
 * Does not require the whole consignment to be FINISHED — other agency flows
 * (NPQS, CDA, …) may still be In Progress after an SLTB reject.
 *
 * Used only for display — the list DTO does not carry review_outcome.
 */
export function isRejectedSltbBlendSheetStep(step: WorkflowNode, allSteps: WorkflowNode[]): boolean {
  if (step.state !== 'COMPLETED') return false
  if ((step.workflowNodeTemplate.name ?? '') !== SLTB_BLEND_SHEET_REVIEW_TITLE) return false
  // Titles are "[SLTB] …" or "[SLTB 3A] …" — match the common "[SLTB" prefix.
  return !allSteps.some((other) => other.id !== step.id && (other.workflowNodeTemplate.name ?? '').startsWith('[SLTB'))
}

/**
 * True when the consignment finished because the blend sheet was rejected
 * (no parallel agency work left — header badge becomes Rejected).
 */
export function isRejectedSltbConsignment(steps: WorkflowNode[], consignmentState?: ConsignmentState): boolean {
  if (consignmentState !== 'FINISHED') return false
  return steps.some((step) => isRejectedSltbBlendSheetStep(step, steps))
}
