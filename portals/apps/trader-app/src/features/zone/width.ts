import { isStepperUiSchema } from '@opennsw/jsonforms-renderers'
import type { ZoneView } from './types'

// The page width for a task. A stepper form (a Categorization with
// options.variant "stepper") puts its step list beside the fields, so it gets
// a wider page than a single-column form, whose fields stay at a readable
// length.
export function zoneWidthClass(task: ZoneView): 'max-w-5xl' | 'max-w-3xl' {
  const stepper = task.view.some((c) => c.type === 'FORM' && isStepperUiSchema(c.payload.uiSchema))
  return stepper ? 'max-w-5xl' : 'max-w-3xl'
}
