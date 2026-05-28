import type { JsonSchema, UISchemaElement } from '@jsonforms/core'

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

export type ZoneComponent =
  | { type: 'FORM'; payload: FormPayload }
  | { type: 'MARKDOWN'; payload: MarkdownPayload }
  | { type: 'REDIRECT'; payload: RedirectPayload }

export type AlertVariant = 'info' | 'success' | 'warning' | 'error'

export type Alert = string | { message: string; title?: string; variant?: AlertVariant }

export type ActionVariant = 'primary' | 'outline' | 'danger'

export type Action =
  | { kind: 'submit_form'; label: string; command: string; variant?: ActionVariant }
  | { kind: 'task_action'; label: string; action: string; variant?: ActionVariant }

export type AuditEntry = {
  timestamp: string
  actor: string
  event: string
  from_state?: string
  to_state?: string
  details?: string
}

export type ZoneView = {
  task_id: string
  task_type: string
  state: string
  alert?: Alert
  actions?: Action[]
  audit?: AuditEntry[]
  view: Record<string, ZoneComponent>
  created_at: string
  updated_at: string
}
