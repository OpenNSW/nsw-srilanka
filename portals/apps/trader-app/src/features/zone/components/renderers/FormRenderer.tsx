import { useEffect, useMemo, useState } from 'react'
import { JsonForms } from '@jsonforms/react'
import { createAjv, type JsonSchema } from '@jsonforms/core'
import { radixRenderers } from '@opennsw/jsonforms-renderers'
import { Button, Callout } from '@radix-ui/themes'
import { ExclamationTriangleIcon } from '@radix-ui/react-icons'
import { useTranslation } from 'react-i18next'
import type { Handle, ZoneRendererProps } from '@/features/zone/types'
import { autoFillForm } from '@/utils/formUtils'
import { getBooleanEnv } from '@/runtimeConfig'

// useDefaults: true lets Ajv populate schema `default` values into the data
// during validation, so defaulted fields (e.g. a single-option country field)
// are pre-filled without the trader touching them.
const ajv = createAjv({ useDefaults: true })

// useDefaults mutates the object it validates, in place, which React cannot
// observe: a memo keyed on that object's reference (requiredErrors) would
// keep the value it computed before the defaults landed, and a required field
// satisfied only by its default would stay flagged as missing. Applying the
// defaults to a private copy up front means state already holds them on the
// first render. Later edits are unaffected — JsonForms builds a new data
// object for every change, so the reference changes then anyway.
function seedWithDefaults(
  schema: JsonSchema | undefined,
  seed: Record<string, unknown> | undefined,
): Record<string, unknown> {
  const seeded = structuredClone(seed ?? {})
  if (schema) ajv.validate(schema, seeded)
  return seeded
}

// A stable empty array, not a fresh `[]` literal at the call site.
//
// @jsonforms/core's JsonFormsStateProvider re-syncs its internal store
// whenever `data`, `additionalErrors`, or `validationMode` changes reference
// (they're all in one effect's dependency array) by dispatching `updateCore`
// — which unconditionally overwrites its own internal data with whatever
// `data` prop it's handed, with no check for whether that's older than
// what it already has. A literal `[]` is a new reference on every render of
// this component for any reason at all, which forces that resync (and a
// possible overwrite of newer internal state with this component's own,
// possibly-stale `data`) far more often than `additionalErrors` actually
// changes. Reusing one stable reference means the dependency only changes
// when the error list itself does. `dataSeed` below is the other half of
// guarding against the same reducer behavior, for when `data` itself is what
// changes.
const EMPTY_ADDITIONAL_ERRORS: RequiredFieldError[] = []

// AJV-shaped error so JsonForms maps it onto the missing control. `message`
// must stay "is a required property" — the radix renderers rewrite that
// exact string to "<label> is required".
type RequiredFieldError = {
  instancePath: string
  schemaPath: string
  keyword: 'required'
  params: { missingProperty: string }
  message: 'is a required property'
}

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
  const { t } = useTranslation()
  // The form owns its data state from mount until submit. payload.data is
  // consumed only as the initial seed: TraderZoneLayout keys Zone by task
  // state, so a state transition unmounts this component and the next mount
  // re-seeds from the fresh payload. Same-state background polls intentionally
  // do *not* clobber in-flight edits — there is no server-side draft to merge
  // back in, so re-syncing payload.data would silently destroy user input.
  const [data, setData] = useState<Record<string, unknown>>(() => seedWithDefaults(payload.schema, payload.data))
  // What actually gets handed to <JsonForms data={...}> — NOT the same as
  // `data` above, and not updated on every onChange either (see the sync
  // effect below). Same UPDATE_CORE reducer behavior EMPTY_ADDITIONAL_ERRORS
  // guards against above, the other half of it: `data` itself legitimately
  // changes on every edit, and re-rendering with a `data` prop that's behind
  // JsonForms's own more recent internal state (e.g. mid-burst, before its
  // 10ms-debounced onChange has reported the latest) gets that newer state
  // silently overwritten. A plain ref updated on every onChange doesn't help
  // — it churns the fed-back reference just as often as state would. Only
  // deliberately lagging behind (see the debounce below) avoids it.
  const [dataSeed, setDataSeed] = useState<Record<string, unknown>>(data)
  const [errors, setErrors] = useState<unknown[]>([])
  const [submitting, setSubmitting] = useState(false)
  const [showErrors, setShowErrors] = useState(false)

  const requiredErrors = useMemo(() => collectRequiredErrors(payload.schema, data), [payload.schema, data])
  // JsonForms merges additionalErrors with native AJV errors. Absent keys
  // already produce a required error; synthesizing another would render
  // "X is required" twice. Only present empty values ("" / []) need a
  // synthetic error — JSON Schema `required` checks presence, not emptiness.
  const additionalRequiredErrors = useMemo(
    () => requiredErrors.filter((error) => isPresentEmpty(data, error)),
    [requiredErrors, data],
  )
  // additionalRequiredErrors is a fresh array reference on every edit even
  // when it's functionally empty (the memo above is keyed on `data`, which
  // changes on every keystroke) — falling back to EMPTY_ADDITIONAL_ERRORS
  // whenever there's nothing to show keeps the prop actually stable once
  // showErrors is true, not just before it.
  const stableAdditionalErrors =
    showErrors && additionalRequiredErrors.length > 0 ? additionalRequiredErrors : EMPTY_ADDITIONAL_ERRORS
  // Catch dataSeed up to data, synchronously, for as long as showErrors is
  // true. showErrors only ever flips true once per session and then stays
  // true (see handleAction below), and for that whole rest of the session
  // validationMode is 'ValidateAndShow' and additionalErrors may be a fresh
  // non-empty reference on every edit (stableAdditionalErrors above) — both
  // are dependencies of JsonForms's own resync effect. Without this, every
  // edit made after one failed submit attempt would keep reintroducing the
  // exact hazard EMPTY_ADDITIONAL_ERRORS/dataSeed otherwise guard against,
  // not just the one showErrors transition. This is React's sanctioned
  // "adjust state during render" pattern: it bails out via Object.is once
  // dataSeed has caught up, so it doesn't cause an extra render on settled
  // data.
  if (showErrors && dataSeed !== data) {
    setDataSeed(data)
  }
  // A FORM zone is editable iff it has at least one legal handle and a
  // dispatch callback; otherwise it renders read-only with no footer. This
  // collapses interactivity, readonly, and button visibility into a single
  // derived fact — the same rule the backend uses to derive Role.
  const isValid = errors.length === 0 && requiredErrors.length === 0
  const interactive = (handles?.length ?? 0) > 0 && onAction !== undefined
  const showAutoFill = interactive && getBooleanEnv('VITE_SHOW_AUTOFILL_BUTTON', false)

  // Re-sync dataSeed to the latest known data — but only once `data` has held
  // still for a while, not on every change. This delay is invisible to the
  // user: it doesn't affect what typing shows (that's JsonForms's own
  // internal state, rendered directly, independent of this prop) or
  // submit-gating (isValid/requiredErrors read `data`, never `dataSeed`) —
  // dataSeed exists solely to give JsonForms's own internal store a safe,
  // settled snapshot to reconcile against on the rare renders (e.g.
  // showErrors toggling) that also touch additionalErrors or validationMode.
  // 500ms is generous on purpose: safety here costs nothing visible, so
  // there's no reason to cut it close.
  useEffect(() => {
    const timeout = setTimeout(() => setDataSeed(data), 500)
    return () => clearTimeout(timeout)
  }, [data])

  const handleAutoFill = () => {
    const next = autoFillForm(payload.schema, data) as Record<string, unknown>
    // A deliberate seed change: apply it to both, the same as a settled
    // onChange round trip would.
    setDataSeed(next)
    setData(next)
  }

  const handleAction = (h: Handle) => {
    if (!onAction) return
    // secondary_action handles (e.g. drafts) accept a partial form.
    // primary_action and danger_action handles require valid data.
    const skipRequired = h.element === 'secondary_action'
    if (!skipRequired && !isValid) {
      // Flipping showErrors changes additionalErrors/validationMode, both
      // dependencies of JsonForms's own resync effect (see
      // EMPTY_ADDITIONAL_ERRORS above) — but the `showErrors && dataSeed !==
      // data` check above already catches dataSeed up to data on the very
      // next render whenever that happens, so this doesn't need its own copy
      // of that logic.
      setShowErrors(true)
      return
    }
    setSubmitting(true)
    void onAction(h.command, data).finally(() => setSubmitting(false))
  }

  return (
    <>
      {interactive && showErrors && requiredErrors.length > 0 && (
        <div className="px-6 pt-6">
          <Callout.Root color="red">
            <Callout.Icon>
              <ExclamationTriangleIcon />
            </Callout.Icon>
            <Callout.Text>{t('tasks.validation.requiredFields')}</Callout.Text>
          </Callout.Root>
        </div>
      )}
      <div className="p-6">
        <JsonForms
          schema={payload.schema}
          uischema={payload.uiSchema}
          data={dataSeed}
          ajv={ajv}
          renderers={radixRenderers}
          readonly={!interactive}
          additionalErrors={stableAdditionalErrors}
          validationMode={showErrors ? 'ValidateAndShow' : 'ValidateAndHide'}
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

// Emits AJV-shaped `required` errors for empty values so JsonForms controls
// show "X is required". JSON Schema `required` only checks presence; empty
// string / empty array would otherwise produce no field error.
function collectRequiredErrors(schema: JsonSchema | undefined, data: unknown, instancePath = ''): RequiredFieldError[] {
  if (!schema || typeof schema !== 'object') return []
  const required = (schema as { required?: string[] }).required
  const properties = (schema as { properties?: Record<string, JsonSchema> }).properties
  const items = (schema as { items?: JsonSchema | JsonSchema[] }).items
  const out: RequiredFieldError[] = []

  if (Array.isArray(required)) {
    const obj = data && typeof data === 'object' && !Array.isArray(data) ? (data as Record<string, unknown>) : undefined
    for (const key of required) {
      if (isEmpty(obj?.[key])) {
        out.push({
          instancePath,
          schemaPath: '#/required',
          keyword: 'required',
          params: { missingProperty: key },
          message: 'is a required property',
        })
      }
    }
  }

  if (properties && data && typeof data === 'object' && !Array.isArray(data)) {
    const obj = data as Record<string, unknown>
    for (const key of Object.keys(properties)) {
      if (obj[key] === undefined) continue
      out.push(...collectRequiredErrors(properties[key], obj[key], `${instancePath}/${key}`))
    }
  }

  if (items && !Array.isArray(items) && Array.isArray(data)) {
    data.forEach((item, index) => {
      out.push(...collectRequiredErrors(items, item, `${instancePath}/${index}`))
    })
  }

  return out
}

// True when the required property exists on the instance but is empty, so
// AJV will not have emitted its own `required` error for that key.
function isPresentEmpty(data: unknown, error: RequiredFieldError): boolean {
  const parent = valueAtPath(data, error.instancePath)
  if (!parent || typeof parent !== 'object' || Array.isArray(parent)) return false
  return Object.prototype.hasOwnProperty.call(parent, error.params.missingProperty)
}

function valueAtPath(data: unknown, instancePath: string): unknown {
  if (!instancePath) return data
  return instancePath
    .split('/')
    .filter(Boolean)
    .reduce<unknown>((current, part) => {
      if (current == null || typeof current !== 'object') return undefined
      return Array.isArray(current) ? current[Number(part)] : (current as Record<string, unknown>)[part]
    }, data)
}

function isEmpty(value: unknown): boolean {
  if (value === undefined || value === null) return true
  if (typeof value === 'string' && value.trim() === '') return true
  if (Array.isArray(value) && value.length === 0) return true
  return false
}
