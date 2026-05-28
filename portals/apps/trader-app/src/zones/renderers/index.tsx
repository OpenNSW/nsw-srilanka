import type { ZoneComponent } from '../types'
import { FormRenderer } from './FormRenderer'
import { MarkdownRenderer } from './MarkdownRenderer'
import { RedirectRenderer } from './RedirectRenderer'
import { UnknownRenderer } from './UnknownRenderer'

export function renderZoneComponent(component: ZoneComponent) {
  switch (component.type) {
    case 'FORM':
      return <FormRenderer payload={component.payload} />
    case 'MARKDOWN':
      return <MarkdownRenderer payload={component.payload} />
    case 'REDIRECT':
      return <RedirectRenderer key={component.payload.checkout_url} payload={component.payload} />
    default:
      return <UnknownRenderer type={(component as { type: string }).type} />
  }
}
