import type { ConsignmentState, WorkflowNode } from '@/features/consignment/types'

/** Exact trader-facing title of the SLTB blend-sheet submit/review step. */
const SLTB_BLEND_SHEET_REVIEW_TITLE = '[SLTB] Submit & Review Blend Sheet'

/**
 * True when this completed blend-sheet review ended the consignment by officer
 * rejection: the case is FINISHED and no later SLTB step was ever opened
 * (approval would have created Pay Levy / sample / lab / final-decision tasks).
 *
 * Used only for display — the list DTO does not carry review_outcome.
 */
export function isRejectedSltbBlendSheetStep(
  step: WorkflowNode,
  allSteps: WorkflowNode[],
  consignmentState?: ConsignmentState,
): boolean {
  if (consignmentState !== 'FINISHED' || step.state !== 'COMPLETED') return false
  if ((step.workflowNodeTemplate.name ?? '') !== SLTB_BLEND_SHEET_REVIEW_TITLE) return false
  return !allSteps.some(
    (other) =>
      other.id !== step.id && (other.workflowNodeTemplate.name ?? '').startsWith('[SLTB]'),
  )
}

/** True when the consignment finished because the blend sheet was rejected. */
export function isRejectedSltbConsignment(
  steps: WorkflowNode[],
  consignmentState?: ConsignmentState,
): boolean {
  return steps.some((step) => isRejectedSltbBlendSheetStep(step, steps, consignmentState))
}
