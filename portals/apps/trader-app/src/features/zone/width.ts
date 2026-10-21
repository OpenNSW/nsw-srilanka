import { isStepperUiSchema } from '@opennsw/jsonforms-renderers'
import type { ZoneView } from './types'

// The width of a task's content under its title. The page itself is as wide
// as every other page; a stepper form (a Categorization with options.variant
// "stepper") puts its step list beside the fields and uses all of it, while a
// single-column form keeps its fields at a readable length.
export function zoneWidthClass(task: ZoneView): 'w-full' | 'max-w-3xl' {
  const stepper = task.view.some((c) => c.type === 'FORM' && isStepperUiSchema(c.payload.uiSchema))
  return stepper ? 'w-full' : 'max-w-3xl'
}
