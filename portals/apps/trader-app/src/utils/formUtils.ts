import { type JsonSchema } from '@jsonforms/core'

// Helper to check if a field should be skipped (readonly or from global context)
const shouldSkipField = (property: any): boolean => {
  // Skip if marked as readOnly
  if (property.readOnly === true) {
    return true
  }
  // Skip if it has x-globalContext.readFrom (value comes from backend)
  if (property['x-globalContext']?.readFrom !== '' && property['x-globalContext']?.readFrom !== undefined) {
    return true
  }
  // Skip x-computed fields: ComputedControl derives these from its configured
  // inputs and rewrites them on every recompute, so any sample value we wrote
  // here would be immediately overwritten anyway.
  if (property['x-computed'] !== undefined) {
    return true
  }
  return false
}

// x-spreadsheet fields are `type: 'object'` with `sheet`/`derivations` sub-
// properties, so the generic object walk would "fill" them with an empty,
// meaningless {sheet: [], derivations: {}}. They need their own generator.
const isSpreadsheetField = (property: unknown): boolean =>
  (property as SpreadsheetSchema | null)?.['x-spreadsheet'] !== undefined

// The slices of an x-spreadsheet schema this generator reads.
interface SpreadsheetSchema {
  'x-spreadsheet'?: { persistSheet?: boolean; columnHeader?: boolean; rowHeader?: boolean }
  'x-evaluate'?: { id?: unknown; label?: unknown }[]
}

// Generic column names for a generated sheet — this is demo data standing in
// for a file nobody uploaded, so it deliberately doesn't try to imitate any
// particular industry's real columns.
const SAMPLE_SHEET_COLUMNS = ['Item', 'Description', 'Quantity', 'Rate', 'Value']
const SAMPLE_SHEET_ROWS = 3

// Builds the same { sheet, derivations } shape SpreadsheetControl persists
// after a real upload, so a spreadsheet-backed form can be filled and
// submitted without a file. The derivations are sample numbers rather than
// values actually computed from `sheet` by the formula engine — for a demo
// button that's fine, and it keeps this helper free of any dependency on the
// renderer package's internals. What does matter, and is honoured here: the
// derivation map is keyed by each x-evaluate `id`, so sibling x-computed
// fields reading `<field>.derivations.<id>.value` resolve to a real number.
const generateSpreadsheetValue = (property: unknown): Record<string, unknown> => {
  const schema = (property ?? {}) as SpreadsheetSchema
  const options = schema['x-spreadsheet'] ?? {}
  const evaluate = Array.isArray(schema['x-evaluate']) ? schema['x-evaluate'] : []

  const derivations: Record<string, unknown> = {}
  evaluate.forEach((entry, index) => {
    if (!entry || typeof entry.id !== 'string') return
    derivations[entry.id] = {
      label: typeof entry.label === 'string' ? entry.label : entry.id,
      value: (index + 1) * 1000,
    }
  })

  const value: Record<string, unknown> = { derivations }

  // persistSheet: false means the real control stores derivations only.
  if (options.persistSheet !== false) {
    const rows = Array.from({ length: SAMPLE_SHEET_ROWS }, (_, r) => [
      `Sample ${r + 1}`,
      `Demo row ${r + 1}`,
      (r + 1) * 10,
      (r + 1) * 100,
      (r + 1) * 1000,
    ])
    // Either header flag makes the persisted sheet records-shaped; with
    // neither, it stays the raw matrix. Mirrors shapeSheet in the renderers.
    value.sheet =
      options.columnHeader || options.rowHeader
        ? rows.map((row) => Object.fromEntries(SAMPLE_SHEET_COLUMNS.map((col, i) => [col, row[i]])))
        : [[...SAMPLE_SHEET_COLUMNS], ...rows]
  }

  return value
}

// The `required` list at one schema level, as a Set. Autofill only ever fills
// required fields — it exists to get a form to a submittable state quickly,
// and filling optional fields just buries the tester in sample values they
// then have to clear out one by one.
const requiredNames = (schema: unknown): Set<string> => {
  const required = (schema as { required?: unknown } | null)?.required
  return new Set<string>(Array.isArray(required) ? (required as string[]) : [])
}

// Generate sample data for a field based on its schema
const generateSampleValue = (property: any, fieldName: string): unknown => {
  // Check if this field should be skipped
  if (shouldSkipField(property)) {
    return undefined
  }

  // Check if there's an example in the property
  if (property.example !== undefined) {
    return property.example
  }

  // Check if there's a description with an example pattern
  if (property.description && typeof property.description === 'string') {
    // Extract example from description if it follows "Example: ..." pattern
    const exampleMatch = property.description.match(/Example:\s*(.+)/i)
    if (exampleMatch) {
      return exampleMatch[1].trim()
    }
  }

  // Handle enum or oneOf (select fields)
  if (property.enum && property.enum.length > 0) {
    return property.enum[0]
  }
  if (property.oneOf && property.oneOf.length > 0) {
    return property.oneOf[0].const
  }

  // Handle by type
  switch (property.type) {
    case 'boolean':
      return true
    case 'number':
    case 'integer':
      if (property.minimum !== undefined) {
        return property.minimum
      }
      if (property.maximum !== undefined) {
        return Math.floor(property.maximum / 2)
      }
      return 100
    case 'string':
      if (property.format === 'email') {
        return 'test@example.com'
      }
      if (property.format === 'date') {
        return new Date().toISOString().split('T')[0]
      }
      if (property.format === 'time') {
        return '10:00:00'
      }
      if (property.format === 'date-time') {
        return new Date().toISOString()
      }
      if (property.format === 'file') {
        // Respect x-file.accept so we don't generate an invalid extension
        // (e.g. an .xlsx-only field should get a .xlsx sample, not .pdf).
        const accept: unknown = property['x-file']?.accept
        const ext =
          typeof accept === 'string'
            ? accept
                .split(',')
                .map((token) => token.trim())
                .find((token) => token.startsWith('.'))
            : undefined
        return `sample_document${ext ?? '.pdf'}`
      }
      if (property.format === 'data-url') {
        // Return a small base64 pixel image as sample file
        return 'data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg=='
      }
      if (property.enum) {
        return property.enum[0]
      }
      // Fall back to a generic sample value
      const label = property.title || fieldName
      return `Sample ${label}`
    case 'object': {
      // A spreadsheet field is an object too, but its contents are a persisted
      // upload result, not ordinary sub-fields — check before the generic walk.
      if (isSpreadsheetField(property)) {
        return generateSpreadsheetValue(property)
      }
      // Recursively generate nested objects, required sub-fields only
      if (property.properties) {
        const required = requiredNames(property)
        const nestedObj: Record<string, unknown> = {}
        for (const [nestedName, nestedProperty] of Object.entries(property.properties)) {
          if (!required.has(nestedName)) continue
          const value = generateSampleValue(nestedProperty, nestedName)
          if (value !== undefined) {
            nestedObj[nestedName] = value
          }
        }
        return nestedObj
      }
      return {}
    }
    case 'array':
      if (property.items) {
        if (Array.isArray(property.items)) {
          return property.items.map((itemSchema: any, idx: number) => generateSampleValue(itemSchema, 'item' + idx))
        }
        if (typeof property.items === 'object') {
          const item = generateSampleValue(property.items, 'item')
          return item !== undefined ? [item] : []
        }
      }
      return []
    default:
      return `Sample ${fieldName}`
  }
}

// Auto-fill empty fields with sample data
export const autoFillForm = (schema: JsonSchema, currentValues: any = {}): any => {
  const newValues = { ...currentValues }

  // Helper to check if a value is empty
  const isEmpty = (val: unknown): boolean =>
    val === undefined || val === null || val === '' || (Array.isArray(val) && val.length === 0)

  // Helper to recursively auto-fill nested objects
  const fillNestedValues = (
    currentSchema: JsonSchema,
    currentValues: Record<string, unknown>,
    path: string[] = [],
  ): Record<string, unknown> => {
    const result = { ...currentValues }

    if (currentSchema.properties) {
      const required = requiredNames(currentSchema)
      for (const [name, property] of Object.entries(currentSchema.properties)) {
        // Required fields only — see requiredNames above for why.
        if (!required.has(name)) {
          continue
        }
        // Skip fields that should not be auto-filled
        if (shouldSkipField(property)) {
          continue
        }

        if (isSpreadsheetField(property)) {
          // Checked before the generic object branch below: a spreadsheet
          // field is an object, but walking its sheet/derivations sub-schema
          // would produce an empty {sheet: [], derivations: {}} that satisfies
          // the schema while meaning nothing.
          if (isEmpty(result[name])) {
            result[name] = generateSpreadsheetValue(property)
          }
        } else if (property.type === 'object' && property.properties) {
          // Recursively fill nested objects
          const nestedValues = (result[name] as Record<string, unknown>) || {}
          result[name] = fillNestedValues(property as JsonSchema, nestedValues, [...path, name])
        } else if (isEmpty(result[name])) {
          // Only fill if the field is empty
          const value = generateSampleValue(property, name)
          if (value !== undefined) {
            result[name] = value
          }
        }
      }
    }

    return result
  }

  return fillNestedValues(schema, newValues)
}
