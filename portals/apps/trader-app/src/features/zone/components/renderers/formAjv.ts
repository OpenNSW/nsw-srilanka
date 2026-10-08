import { createAjv, defaultErrorTranslator, type ErrorTranslator, type JsonSchema } from '@jsonforms/core'

// $data: true — needed for formatMaximum: { $data: "1/<sibling>" } date-order rules in schemas.
export const formAjv = createAjv({ useDefaults: true, $data: true })

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
