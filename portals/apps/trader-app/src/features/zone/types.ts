import type { JsonSchema, UISchemaElement } from '@jsonforms/core'
import type { ReactNode } from 'react'

export type FormPayload = {
  schema: JsonSchema
  uiSchema?: UISchemaElement
  data?: Record<string, unknown>
  readonly?: boolean
}

export type MarkdownPayload = {
  content: string
}

export type RedirectPayload = {
  checkout_url: string
  content: string
}

export type AlertVariant = 'info' | 'success' | 'warning' | 'error'

export type Alert = string | { message: string; title?: string; variant?: AlertVariant }

// Handle is one operation as the trader-app receives it: command identifies
// what to dispatch, label is the user-facing text, element is an identifier
// owned by this zone's renderer (e.g. 'primary_action', 'secondary_action'
// for a FORM zone). Dispatch behavior — whether to gather form data,
// validation gating — is decided by the renderer based on element, not by
// any field on the handle itself.
export type Handle = {
  command: string
  label: string
  element?: string
}

type ZoneComponentBase = {
  handles?: Handle[]
}

export type ZoneComponent =
  | (ZoneComponentBase & { type: 'FORM'; payload: FormPayload })
  | (ZoneComponentBase & { type: 'MARKDOWN'; payload: MarkdownPayload })
  | (ZoneComponentBase & { type: 'REDIRECT'; payload: RedirectPayload })

export type AuditEntry = {
  timestamp: string
  actor: string
  event: string
  from_state?: string
  to_state?: string
  details?: string
}

// ZoneView is the wire shape served by GET /api/v1/tasks/{id}. There is no
// separate top-level actions list — operations ship inside their claiming
// zone's handles (joined to state legality by the backend assembler).
export type ZoneView = {
  task_id: string
  task_type: string
  state: string
  alert?: Alert
  audit?: AuditEntry[]
  view: Record<string, ZoneComponent>
  created_at: string
  updated_at: string
}

export type ZoneRendererProps<T extends ZoneComponent['type']> = {
  payload: Extract<ZoneComponent, { type: T }>['payload']
}

export type ZoneRenderer<T extends ZoneComponent['type']> = (props: ZoneRendererProps<T>) => ReactNode
