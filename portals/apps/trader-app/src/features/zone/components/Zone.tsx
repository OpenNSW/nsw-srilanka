import type { ZoneAction, ZoneComponent } from '@/features/zone/types'
import { isStepperUiSchema } from '@opennsw/jsonforms-renderers'
import { renderZoneComponent } from './renderers'

type Props = {
  component: ZoneComponent
  onAction?: ZoneAction
}

// Zone is the chrome around every rendered zone: section header (the zone's
// title, if available) + white rounded box. It is intentionally
// projector-agnostic — it forwards the dispatch callback verbatim and lets
// renderZoneComponent fan out by type. A zone is interactive iff onAction is
// provided AND the inner renderer finds legal handles on its component; that
// derivation lives in the renderer (today: FormRenderer), not here.
export function Zone({ component, onAction }: Props) {
  return (
    <section className="space-y-2">
      {component.title && <h2 className="text-xs font-semibold text-foreground-muted">{component.title}</h2>}
      {/* A stepper form draws its own cards: the step list and the step. */}
      {component.type === 'FORM' && isStepperUiSchema(component.payload.uiSchema) ? (
        renderZoneComponent(component, { onAction })
      ) : (
        <div className="bg-app-surface rounded-2xl shadow-md">{renderZoneComponent(component, { onAction })}</div>
      )}
    </section>
  )
}
