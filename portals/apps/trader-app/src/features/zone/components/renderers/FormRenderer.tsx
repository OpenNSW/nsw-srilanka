import { useState } from 'react'
import { JsonForms } from '@jsonforms/react'
import { radixRenderers } from '@opennsw/jsonforms-renderers'
import { Button, Text } from '@radix-ui/themes'
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
  const [errors, setErrors] = useState<ValidationError[]>([])
  const [submitting, setSubmitting] = useState(false)
  const [showErrors, setShowErrors] = useState(false)
  // Populated only when a submit is actually blocked, so the user gets a named
  // reason instead of a dead button. Cleared on the next edit — see onChange.
  const [blocked, setBlocked] = useState<string[] | null>(null)

  // A FORM zone is editable iff it has at least one legal handle and a
  // dispatch callback; otherwise it renders read-only with no footer. This
  // collapses interactivity, readonly, and button visibility into a single
  // derived fact — the same rule the backend uses to derive Role.
  const missingRequired = missingRequiredPaths(payload.schema, data)
  const isValid = errors.length === 0 && missingRequired.length === 0
  const interactive = (handles?.length ?? 0) > 0 && onAction !== undefined
  const showAutoFill = interactive && getBooleanEnv('VITE_SHOW_AUTOFILL_BUTTON', false)

  const handleAutoFill = () => {
    const next = autoFillForm(payload.schema, data) as Record<string, unknown>
    setData(next)
  }

  const handleAction = (h: Handle) => {
    if (!onAction) return
    const isSubmitAction = h.element !== 'secondary_action'
    if (isSubmitAction && !isValid) {
      // Name the reasons before aborting. Flipping validationMode alone is not
      // enough: it only paints errors on fields that actually render a
      // control, so a required property with no uiSchema Control (or one whose
      // renderer doesn't display errors) would otherwise block the submit with
      // no feedback anywhere — a dead button and a silent console.
      const reasons = describeBlockers(missingRequired, errors)
      setBlocked(reasons)
      setShowErrors(true)
      console.warn('[FormRenderer] submit blocked by validation:', reasons)
      return
    }
    setBlocked(null)
    setSubmitting(true)
    void onAction(h.command, data).finally(() => setSubmitting(false))
  }

  return (
    <>
      {blocked && blocked.length > 0 && (
        <div className="px-6 pt-6">
          <div className="rounded-lg border border-red-6 bg-red-2 px-4 py-3">
            <Text size="2" color="red" weight="medium">
              This form can't be submitted yet:
            </Text>
            <ul className="mt-1 list-disc pl-5">
              {blocked.map((reason) => (
                <li key={reason}>
                  <Text size="2" color="red">
                    {reason}
                  </Text>
                </li>
              ))}
            </ul>
          </div>
        </div>
      )}
      <div className="p-6">
        <JsonForms
          schema={payload.schema}
          uischema={payload.uiSchema}
          data={data}
          renderers={radixRenderers}
          readonly={!interactive}
          validationMode={showErrors ? 'ValidateAndShow' : 'ValidateAndHide'}
          onChange={({ data, errors }) => {
            const next = (data ?? {}) as Record<string, unknown>
            setData(next)
            setErrors((errors ?? []) as ValidationError[])
            // Any edit invalidates the previously-reported blockers; the next
            // submit attempt recomputes them from scratch.
            setBlocked(null)
          }}
        />
      </div>
      {interactive && (
        <FormActionBar
          handles={handles ?? []}
          onAction={handleAction}
          onAutoFill={showAutoFill ? handleAutoFill : undefined}
          submitting={submitting}
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
}: {
  handles: Handle[]
  onAction: (h: Handle) => void
  onAutoFill?: () => void
  submitting: boolean
}) {
  return (
    <div className="sticky bottom-0 border-t border-border bg-app-surface/95 backdrop-blur rounded-b-lg shadow-[0_-4px_12px_-8px_rgba(0,0,0,0.08)]">
      <div className="px-6 py-4 flex items-center gap-3">
        {onAutoFill && (
          <Button type="button" variant="soft" color="purple" size="3" onClick={onAutoFill} disabled={submitting}>
            Demo - Auto Fill
          </Button>
        )}
        <div className="flex-1" />
        {handles.map((h) => (
          <HandleButton key={h.command} handle={h} onClick={onAction} submitting={submitting} />
        ))}
      </div>
    </div>
  )
}

function HandleButton({
  handle,
  onClick,
  submitting,
}: {
  handle: Handle
  onClick: (h: Handle) => void
  submitting: boolean
}) {
  const style = (handle.element && FORM_ELEMENT_CATALOG[handle.element]) || { variant: 'solid' as const }
  const disabled = submitting
  return (
    <Button onClick={() => onClick(handle)} size="3" variant={style.variant} color={style.color} disabled={disabled}>
      {submitting ? 'Submitting...' : handle.label}
    </Button>
  )
}

// The subset of ajv's ErrorObject this file actually reads. Declared locally
// rather than imported so the app doesn't take a direct dependency on ajv —
// JSONForms owns that instance, we only format what it hands back.
type ValidationError = {
  instancePath?: string
  message?: string
  params?: { missingProperty?: string }
}

// Walks the schema's `required` arrays and collects the dotted path of every
// required key that isn't filled in. Same traversal and emptiness rules as the
// boolean check it replaces — it just names the offenders instead of
// short-circuiting, so a blocked submit can say what's wrong.
function missingRequiredPaths(schema: JsonSchema | undefined, data: unknown, prefix = ''): string[] {
  if (!schema || typeof schema !== 'object') return []
  const required = (schema as { required?: string[] }).required
  const properties = (schema as { properties?: Record<string, JsonSchema> }).properties
  const missing: string[] = []
  const at = (key: string) => (prefix ? `${prefix}.${key}` : key)

  if (Array.isArray(required)) {
    if (!data || typeof data !== 'object') {
      return required.map(at)
    }
    const obj = data as Record<string, unknown>
    for (const key of required) {
      if (isEmpty(obj[key])) missing.push(at(key))
    }
  }

  if (properties && data && typeof data === 'object') {
    const obj = data as Record<string, unknown>
    for (const key of Object.keys(properties)) {
      if (obj[key] !== undefined) {
        missing.push(...missingRequiredPaths(properties[key], obj[key], at(key)))
      }
    }
  }

  return missing
}

// Merges the two independent sources of "not submittable" into one de-duplicated,
// human-readable list: the required-path walk above (which also covers keys the
// uiSchema never renders) and ajv's own errors (which cover format, type, and
// anything inside array items the walk above doesn't descend into).
function describeBlockers(missingRequired: string[], errors: ValidationError[]): string[] {
  const reasons = missingRequired.map((path) => `${path} is required`)

  for (const err of errors) {
    const path = (err.instancePath ?? '').replace(/^\//, '').replace(/\//g, '.')
    const missingProperty = err.params?.missingProperty
    if (missingProperty) {
      reasons.push(`${path ? `${path}.` : ''}${missingProperty} is required`)
      continue
    }
    reasons.push(`${path || 'form'} ${err.message ?? 'is invalid'}`)
  }

  return [...new Set(reasons)]
}

function isEmpty(value: unknown): boolean {
  if (value === undefined || value === null) return true
  if (typeof value === 'string' && value.trim() === '') return true
  if (Array.isArray(value) && value.length === 0) return true
  return false
}
