import { JsonForms } from '@jsonforms/react'
import { radixRenderers } from '@opennsw/jsonforms-renderers'
import type { ZoneRendererProps } from './types'

export function FormRenderer({ payload }: ZoneRendererProps<'FORM'>) {
  return (
    <JsonForms
      schema={payload.schema}
      uischema={payload.uiSchema}
      data={payload.data ?? {}}
      renderers={radixRenderers}
      readonly={payload.readonly ?? false}
    />
  )
}
