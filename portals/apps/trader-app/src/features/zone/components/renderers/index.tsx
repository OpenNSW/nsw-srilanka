import type { ZoneComponent } from '@/features/zone/types'
import { FormRenderer } from './FormRenderer'
import { MarkdownRenderer } from './MarkdownRenderer'
import { RedirectRenderer } from './RedirectRenderer'
import { UnknownRenderer } from './UnknownRenderer'

// RenderOptions carries the dispatch context for interactive renderers.
// onAction is provided by WorkspaceZone (the INTERACTIVE call site) and
// omitted by PresentationalZone — renderers that consume it (today: FORM)
// become editable iff it's present. Renderers that don't (MARKDOWN,
// REDIRECT) ignore the options uniformly; no type-specific branching lives
// in the caller.
export type RenderOptions = {
  onAction?: (command: string, data: Record<string, unknown>) => Promise<void>
}

export function renderZoneComponent(component: ZoneComponent, options: RenderOptions = {}) {
  switch (component.type) {
    case 'FORM':
      return <FormRenderer payload={component.payload} handles={component.handles} onAction={options.onAction} />
    case 'MARKDOWN':
      return <MarkdownRenderer payload={component.payload} />
    case 'REDIRECT':
      return <RedirectRenderer key={component.payload.checkout_url} payload={component.payload} />
    default:
      return <UnknownRenderer type={(component as { type: string }).type} />
  }
}
