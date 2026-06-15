import type { ZoneComponent } from './types'
import { renderZoneComponent } from './renderers'

type Props = {
  name: string
  component: ZoneComponent
  onAction?: (command: string, data: Record<string, unknown>) => Promise<void>
}

// Zone is the chrome around every rendered zone: section header + white
// rounded box. It is intentionally projector-agnostic — it forwards the
// dispatch callback verbatim and lets renderZoneComponent fan out by type.
// A zone is interactive iff onAction is provided AND the inner renderer
// finds legal handles on its component; that derivation lives in the
// renderer (today: FormRenderer), not here.
export function Zone({ name, component, onAction }: Props) {
  return (
    <section className="space-y-2">
      <span className="text-xs font-semibold uppercase tracking-widest text-foreground-subtle">{name}</span>
      <div className="bg-background rounded-lg shadow-sm border border-border">
        {renderZoneComponent(component, { onAction })}
      </div>
    </section>
  )
}
