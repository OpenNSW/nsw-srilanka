import type { ComponentType } from 'react'
import type { ZoneComponent } from '../types'
import { FormRenderer } from './FormRenderer'
import { MarkdownRenderer } from './MarkdownRenderer'
import { UnknownRenderer } from './UnknownRenderer'
import type { ZoneRenderer } from './types'

type Registry = { [K in ZoneComponent['type']]: ZoneRenderer<K> }

// Add new component types here. `satisfies` ensures every variant in the
// ZoneComponent discriminated union has a renderer — missing one fails the
// build instead of falling through to UnknownRenderer at runtime.
const REGISTRY = {
  FORM: FormRenderer,
  MARKDOWN: MarkdownRenderer,
} satisfies Registry

export function renderZoneComponent(component: ZoneComponent) {
  const Renderer = REGISTRY[component.type] as ComponentType<{ payload: typeof component.payload }> | undefined
  if (!Renderer) return <UnknownRenderer type={component.type} />
  return <Renderer payload={component.payload} />
}
