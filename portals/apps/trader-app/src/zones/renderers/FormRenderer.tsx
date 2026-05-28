import { forwardRef, useEffect, useImperativeHandle, useRef, useState } from 'react'
import { JsonForms } from '@jsonforms/react'
import { radixRenderers } from '@opennsw/jsonforms-renderers'
import type { JsonSchema } from '@jsonforms/core'
import type { ZoneRendererProps } from './types'
import { autoFillForm } from '../../utils/formUtils'

export type FormHandle = {
  getData: () => Record<string, unknown>
  autoFill: () => void
}

type Props = ZoneRendererProps<'FORM'> & {
  onValidityChange?: (isValid: boolean) => void
}

export const FormRenderer = forwardRef<FormHandle, Props>(function FormRenderer({ payload, onValidityChange }, ref) {
  const [data, setData] = useState<Record<string, unknown>>(payload.data ?? {})
  const dataRef = useRef(data)
  dataRef.current = data

  useImperativeHandle(
    ref,
    () => ({
      getData: () => dataRef.current,
      autoFill: () => {
        const next = autoFillForm(payload.schema, dataRef.current) as Record<string, unknown>
        setData(next)
        onValidityChange?.(isFormValid(payload.schema, next, []))
      },
    }),
    [onValidityChange, payload.schema],
  )

  useEffect(() => {
    onValidityChange?.(isFormValid(payload.schema, data, []))
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  return (
    <JsonForms
      schema={payload.schema}
      uischema={payload.uiSchema}
      data={data}
      renderers={radixRenderers}
      readonly={payload.readonly ?? false}
      onChange={({ data, errors }) => {
        const next = (data ?? {}) as Record<string, unknown>
        setData(next)
        onValidityChange?.(isFormValid(payload.schema, next, errors ?? []))
      }}
    />
  )
})

function isFormValid(schema: JsonSchema, data: Record<string, unknown>, errors: unknown[]): boolean {
  if (errors.length > 0) return false
  return allRequiredFilled(schema, data)
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
