import { createAjv, defaultErrorTranslator, type ErrorTranslator, type JsonSchema } from '@jsonforms/core'

// $data: true — needed for formatMaximum: { $data: "1/<sibling>" } date-order rules in schemas.
export const formAjv = createAjv({ useDefaults: true, $data: true })

function localISODate(now = new Date()): string {
  const y = now.getFullYear()
  const m = String(now.getMonth() + 1).padStart(2, '0')
  const d = String(now.getDate()).padStart(2, '0')
  return `${y}-${m}-${d}`
}

// notBefore: "today" rejects a date before the trader's local calendar day.
// Today is allowed. Unknown schema values are ignored so a typo does not fail every date.
formAjv.addKeyword({
  keyword: 'notBefore',
  type: 'string',
  schemaType: 'string',
  validate: (schemaVal: string, data: unknown) => {
    if (schemaVal !== 'today') return true
    if (typeof data !== 'string' || data.length < 10) return true
    return data.slice(0, 10) >= localISODate()
  },
})

function parentObjectSchema(root: JsonSchema, instancePath: string): JsonSchema | undefined {
  const segments = instancePath.split('/').filter(Boolean)
  if (segments.length <= 1) return root
  let current: JsonSchema | undefined = root
  for (const seg of segments.slice(0, -1)) {
    if (!current || typeof current !== 'object' || !current.properties?.[seg]) return undefined
    current = current.properties[seg]
  }
  return current
}

function siblingTitleFromDataRef(
  rootSchema: JsonSchema | undefined,
  instancePath: string,
  dataRef: unknown,
): string | undefined {
  if (typeof dataRef !== 'string') return undefined
  const match = /^\d+\/(.+)$/.exec(dataRef)
  if (!match || !rootSchema) return undefined
  const siblingKey = match[1]
  const parent = parentObjectSchema(rootSchema, instancePath) ?? rootSchema
  const props = parent.properties
  if (!props || typeof props !== 'object') return siblingKey
  const sibling = props[siblingKey] as JsonSchema | undefined
  if (sibling && typeof sibling === 'object' && typeof sibling.title === 'string') return sibling.title
  return siblingKey
}

export function createFormErrorTranslator(rootSchema: JsonSchema | undefined): ErrorTranslator {
  return (error, translate, uischema) => {
    if (error.keyword === 'notBefore') {
      const title =
        error.parentSchema &&
        typeof error.parentSchema === 'object' &&
        typeof (error.parentSchema as JsonSchema).title === 'string'
          ? (error.parentSchema as JsonSchema).title
          : 'This date'
      return `${title} cannot be before today`
    }
    if (error.keyword === 'formatMaximum' && error.parentSchema && typeof error.parentSchema === 'object') {
      const selfTitle =
        typeof (error.parentSchema as JsonSchema).title === 'string'
          ? (error.parentSchema as JsonSchema).title
          : 'This date'
      const dataRef = (error.parentSchema as { formatMaximum?: { $data?: unknown } }).formatMaximum?.$data
      const otherTitle = siblingTitleFromDataRef(rootSchema, error.instancePath, dataRef)
      if (otherTitle) {
        return `${selfTitle} cannot be later than ${otherTitle}`
      }
    }
    return defaultErrorTranslator(error, translate, uischema)
  }
}
