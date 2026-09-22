import { useMemo, useState } from 'react'
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
// whenever the `data`, `additionalErrors`, or `validationMode` props it's
// given change reference (they're all in one effect's dependency array) by
// dispatching `updateCore` — which unconditionally overwrites its own
// internal data with whatever `data` prop it's handed, with no check for
// whether that's older than what it already has. Two things below keep both
// of those props from changing reference except when they genuinely must:
// this constant (so an empty error list is always the same reference), and
// `stableAdditionalErrors`'s content-based memoization for when it's
// non-empty (see its own comment, further down).
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
  // `data` above, and not updated on every onChange either (see the
  // lastFed sync below). Same UPDATE_CORE reducer behavior
  // EMPTY_ADDITIONAL_ERRORS guards against above, the other half of it:
  // `data` itself legitimately changes on every edit, and re-rendering with
  // a `data` prop that's behind JsonForms's own more recent internal state
  // (e.g. mid-burst, before its 10ms-debounced onChange has reported the
  // latest) gets that newer state silently overwritten. Only resyncing at
  // the exact moments JsonForms's resync effect is about to fire anyway
  // (see lastFed below) avoids it, rather than resyncing continuously or
  // on a wall-clock timer.
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
  // when its CONTENT is unchanged (the memo above is keyed on `data`, which
  // changes on every keystroke). additionalErrors is a dependency of
  // JsonForms's own resync effect (see EMPTY_ADDITIONAL_ERRORS above), so an
  // unstable reference here would force that resync far more often than the
  // missing-fields set actually changes — reusing the previous array
  // whenever the set is unchanged keeps the reference, and therefore the
  // resync, tied to genuine content changes only. State, not a ref: this
  // project's lint rules (React Compiler-compatible) disallow touching a
  // ref's `.current` during render, so "remember what render-N computed" has
  // to go through the same sanctioned "adjust state during render" pattern
  // used below for lastFed, not a ref-based memo.
  const [prevStableErrors, setPrevStableErrors] = useState<RequiredFieldError[]>(EMPTY_ADDITIONAL_ERRORS)
  const candidateErrors =
    showErrors && additionalRequiredErrors.length > 0 ? additionalRequiredErrors : EMPTY_ADDITIONAL_ERRORS
  const stableAdditionalErrors =
    missingFieldsSignature(candidateErrors) === missingFieldsSignature(prevStableErrors)
      ? prevStableErrors
      : candidateErrors
  if (stableAdditionalErrors !== prevStableErrors) {
    setPrevStableErrors(stableAdditionalErrors)
  }

  const validationMode: 'ValidateAndShow' | 'ValidateAndHide' = showErrors ? 'ValidateAndShow' : 'ValidateAndHide'

  // Catch dataSeed up to data, synchronously, exactly when additionalErrors
  // or validationMode is about to change — the only moments JsonForms's
  // resync effect actually fires (see EMPTY_ADDITIONAL_ERRORS above).
  // Outside of those moments dataSeed simply stays put: JsonForms's own
  // internal reducer is the source of truth for what's being typed
  // regardless of what this prop holds, so there's nothing to gain from
  // resyncing more often, and every extra resync is one more chance to feed
  // back a snapshot that's behind a still-in-flight async write (e.g. an
  // XML import triggering a SpreadsheetControl formula evaluation — see
  // "Residual risk" below). This is React's sanctioned "adjust state during
  // render" pattern: both setState calls below bail out via Object.is once
  // caught up, so there's no extra render once settled.
  //
  // Residual risk, deliberately accepted: at the exact render this fires,
  // `data` can still be up to @jsonforms/react's own ~10ms onChange debounce
  // behind JsonForms's true internal state — an irreducible floor from
  // outside this codebase, not fixable here. Verified live against the SLTB
  // blend sheet form (XML import -> SpreadsheetControl -> ComputedControl
  // cascade): clicking Submit at realistic human timing (>=100ms after
  // starting the upload) never corrupted the final computed totals across
  // repeated runs; only an unrealistic zero-delay, script-driven click
  // (impossible for an actual person, who needs time to see the file dialog
  // close and move to the button) hit this floor, and rarely even then.
  const [lastFed, setLastFed] = useState<{
    additionalErrors: RequiredFieldError[]
    validationMode: 'ValidateAndShow' | 'ValidateAndHide'
  }>({ additionalErrors: EMPTY_ADDITIONAL_ERRORS, validationMode: 'ValidateAndHide' })
  if (lastFed.additionalErrors !== stableAdditionalErrors || lastFed.validationMode !== validationMode) {
    if (dataSeed !== data) setDataSeed(data)
    setLastFed({ additionalErrors: stableAdditionalErrors, validationMode })
  }

  // A FORM zone is editable iff it has at least one legal handle and a
  // dispatch callback; otherwise it renders read-only with no footer. This
  // collapses interactivity, readonly, and button visibility into a single
  // derived fact — the same rule the backend uses to derive Role.
  const isValid = errors.length === 0 && requiredErrors.length === 0
  const interactive = (handles?.length ?? 0) > 0 && onAction !== undefined
  const showAutoFill = interactive && getBooleanEnv('VITE_SHOW_AUTOFILL_BUTTON', false)

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
      // Flipping showErrors changes validationMode, a dependency of
      // JsonForms's own resync effect (see EMPTY_ADDITIONAL_ERRORS above) —
      // but the lastFed check above already catches dataSeed up to data on
      // the very next render whenever that happens, so this doesn't need
      // its own copy of that logic.
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
          validationMode={validationMode}
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

// A cheap content key for a required-errors list: `instancePath` and
// `missingProperty` are the only fields that ever vary between calls
// (schemaPath/keyword/message are the same literals every time), so this is
// enough to tell "the missing-fields set is unchanged" from "it changed" —
// see stableAdditionalErrors, which uses it to reuse the previous array
// reference whenever the set hasn't moved.
function missingFieldsSignature(errors: RequiredFieldError[]): string {
  return errors.map((error) => `${error.instancePath}:${error.params.missingProperty}`).join(',')
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
