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
  return false
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
      if (property.format === 'date-time') {
        return new Date().toISOString()
      }
      if (property.format === 'time') {
        return '10:00:00'
      }
      if (property.format === 'file') {
        return 'sample_document.pdf'
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
    case 'object':
      // Recursively generate nested objects
      if (property.properties) {
        const nestedObj: Record<string, unknown> = {}
        for (const [nestedName, nestedProperty] of Object.entries(property.properties)) {
          const value = generateSampleValue(nestedProperty, nestedName)
          if (value !== undefined) {
            nestedObj[nestedName] = value
          }
        }
        return nestedObj
      }
      return {}
    case 'array':
      if (property.items && typeof property.items === 'object') {
        const itemVal = generateSampleValue(property.items, `${fieldName}_item`)
        if (itemVal !== undefined) {
          return [itemVal]
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
  const isEmpty = (val: unknown): boolean => {
    return val === undefined || val === null || val === ''
  }

  // Helper to recursively auto-fill nested objects
  const fillNestedValues = (
    currentSchema: JsonSchema,
    currentValues: Record<string, unknown>,
    path: string[] = [],
  ): Record<string, unknown> => {
    const result = { ...currentValues }

    if (currentSchema.properties) {
      for (const [name, property] of Object.entries(currentSchema.properties)) {
        // Skip fields that should not be auto-filled
        if (shouldSkipField(property)) {
          continue
        }

        if (property.type === 'object' && property.properties) {
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
