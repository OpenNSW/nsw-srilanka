import type { ZoneAction, ZoneComponent } from '@/features/zone/types'
import { FormRenderer } from './FormRenderer'
import { MarkdownRenderer } from './MarkdownRenderer'
import { RedirectRenderer } from './RedirectRenderer'
import { UnknownRenderer } from './UnknownRenderer'

// RenderOptions carries the dispatch context for interactive renderers. A
// zone is interactive iff onAction is present: a FORM becomes editable, and a
// REDIRECT sends the browser to its checkout. MARKDOWN ignores the options; no
// type-specific branching lives in the caller.
export type RenderOptions = {
  onAction?: ZoneAction
}

export function renderZoneComponent(component: ZoneComponent, options: RenderOptions = {}) {
  switch (component.type) {
    case 'FORM':
      return <FormRenderer payload={component.payload} handles={component.handles} onAction={options.onAction} />
    case 'MARKDOWN':
      return <MarkdownRenderer payload={component.payload} />
    case 'REDIRECT':
      return (
        <RedirectRenderer
          key={component.payload.checkout_url}
          payload={component.payload}
          interactive={options.onAction !== undefined}
        />
      )
    default:
      return <UnknownRenderer type={(component as { type: string }).type} />
  }
}
