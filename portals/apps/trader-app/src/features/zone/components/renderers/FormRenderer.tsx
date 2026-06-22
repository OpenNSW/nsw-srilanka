import { useState } from 'react'
import { JsonForms } from '@jsonforms/react'
import { radixRenderers } from '@opennsw/jsonforms-renderers'
import { Button } from '@radix-ui/themes'
import type { JsonSchema } from '@jsonforms/core'
import type { Handle, ZoneRendererProps } from '@/features/zone/types'
import { autoFillForm } from '@/utils/formUtils'
import { getBooleanEnv } from '@/runtimeConfig'

type Props = ZoneRendererProps<'FORM'> & {
  // handles, when non-empty, render as physical controls in the form's own
  // footer. Element identifiers are resolved against this renderer's
  // catalog (see FORM_ELEMENT_CATALOG below).
  handles?: Handle[]
  // onAction fires when the user activates a handle. The renderer extracts
  // its own form data and passes it alongside the command. Validation
  // gating is internal — disabled handles cannot fire. The form is
  // editable iff both handles and onAction are provided; otherwise it
  // renders read-only.
  onAction?: (command: string, data: Record<string, unknown>) => Promise<void>
}

// FORM_ELEMENT_CATALOG is this renderer's published list of interactive
// element identifiers and their visual treatment. Handles reference these by
// name via Handle.element; unknown identifiers fall back to a plain solid
// button so the action still dispatches.
const FORM_ELEMENT_CATALOG: Record<string, { variant: 'solid' | 'outline'; color?: 'red' }> = {
  primary_action: { variant: 'solid' },
  secondary_action: { variant: 'outline' },
  danger_action: { variant: 'solid', color: 'red' },
}

export function FormRenderer({ payload, handles, onAction }: Props) {
  // The form owns its data state from mount until submit. payload.data is
  // consumed only as the initial seed: TraderZoneLayout keys Zone by task
  // state, so a state transition unmounts this component and the next mount
  // re-seeds from the fresh payload. Same-state background polls intentionally
  // do *not* clobber in-flight edits — there is no server-side draft to merge
  // back in, so re-syncing payload.data would silently destroy user input.
  const [data, setData] = useState<Record<string, unknown>>(payload.data ?? {})
  const [errors, setErrors] = useState<unknown[]>([])
  const [submitting, setSubmitting] = useState(false)

  // A FORM zone is editable iff it has at least one legal handle and a
  // dispatch callback; otherwise it renders read-only with no footer. This
  // collapses interactivity, readonly, and button visibility into a single
  // derived fact — the same rule the backend uses to derive Role.
  const isValid = errors.length === 0 && allRequiredFilled(payload.schema, data)
  const interactive = (handles?.length ?? 0) > 0 && onAction !== undefined
  const showAutoFill = interactive && getBooleanEnv('VITE_SHOW_AUTOFILL_BUTTON', false)

  const handleAutoFill = () => {
    const next = autoFillForm(payload.schema, data) as Record<string, unknown>
    setData(next)
  }

  const handleAction = (h: Handle) => {
    if (!onAction) return
    setSubmitting(true)
    void onAction(h.command, data).finally(() => setSubmitting(false))
  }

  return (
    <>
      <div className="p-6">
        <JsonForms
          schema={payload.schema}
          uischema={payload.uiSchema}
          data={data}
          renderers={radixRenderers}
          readonly={!interactive}
          onChange={({ data, errors }) => {
            const next = (data ?? {}) as Record<string, unknown>
            setData(next)
            setErrors(errors ?? [])
          }}
        />
      </div>
      {interactive && (
        <FormActionBar
          handles={handles ?? []}
          onAction={handleAction}
          onAutoFill={showAutoFill ? handleAutoFill : undefined}
          submitting={submitting}
          canSubmit={isValid}
        />
      )}
    </>
  )
}

function FormActionBar({
  handles,
  onAction,
  onAutoFill,
  submitting,
  canSubmit,
}: {
  handles: Handle[]
  onAction: (h: Handle) => void
  onAutoFill?: () => void
  submitting: boolean
  canSubmit: boolean
}) {
  return (
    <div className="sticky bottom-0 border-t border-border bg-app-bg/95 backdrop-blur rounded-b-lg shadow-[0_-4px_12px_-8px_rgba(0,0,0,0.08)]">
      <div className="px-6 py-4 flex items-center gap-3">
        {onAutoFill && (
          <Button type="button" variant="soft" color="purple" size="3" onClick={onAutoFill} disabled={submitting}>
            Demo - Auto Fill
          </Button>
        )}
        <div className="flex-1" />
        {handles.map((h) => (
          <HandleButton key={h.command} handle={h} onClick={onAction} submitting={submitting} canSubmit={canSubmit} />
        ))}
      </div>
    </div>
  )
}

function HandleButton({
  handle,
  onClick,
  submitting,
  canSubmit,
}: {
  handle: Handle
  onClick: (h: Handle) => void
  submitting: boolean
  canSubmit: boolean
}) {
  const style = (handle.element && FORM_ELEMENT_CATALOG[handle.element]) || { variant: 'solid' as const }
  const disabled = submitting || !canSubmit
  return (
    <Button onClick={() => onClick(handle)} size="3" variant={style.variant} color={style.color} disabled={disabled}>
      {submitting ? 'Submitting...' : handle.label}
    </Button>
  )
}

// Walks the schema's `required` arrays and checks each path against the data.
// Treats undefined, null, empty string, and empty array as "missing".
function allRequiredFilled(schema: JsonSchema | undefined, data: unknown): boolean {
  if (!schema || typeof schema !== 'object') return true
  const required = (schema as { required?: string[] }).required
  const properties = (schema as { properties?: Record<string, JsonSchema> }).properties

  if (Array.isArray(required) && data && typeof data === 'object') {
    const obj = data as Record<string, unknown>
    for (const key of required) {
      if (isEmpty(obj[key])) return false
    }
  }

  if (properties && data && typeof data === 'object') {
    const obj = data as Record<string, unknown>
    for (const key of Object.keys(properties)) {
      if (!allRequiredFilled(properties[key], obj[key])) return false
    }
  }

  return true
}

function isEmpty(value: unknown): boolean {
  if (value === undefined || value === null) return true
  if (typeof value === 'string' && value.trim() === '') return true
  if (Array.isArray(value) && value.length === 0) return true
  return false
}
