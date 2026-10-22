import { createAjv, defaultErrorTranslator, type ErrorTranslator, type JsonSchema } from '@jsonforms/core'

type JsonFormsAjv = ReturnType<typeof createAjv>

type DateFormat = {
  validate?: (data: string) => boolean
  compare?: (data: string, compareTo: string) => number
}

/** Local calendar day, YYYY-MM-DD. */
function localISODate(now = new Date()): string {
  const y = now.getFullYear()
  const m = String(now.getMonth() + 1).padStart(2, '0')
  const d = String(now.getDate()).padStart(2, '0')
  return `${y}-${m}-${d}`
}

/** `formatMinimum: "today"` is the local day. A fixed date keeps Ajv's compare. */
function withRelativeDates<T extends JsonFormsAjv>(ajv: T): T {
  const formats = ajv.formats as Record<string, DateFormat | string | undefined>
  for (const name of ['date', 'date-time']) {
    const base = formats[name]
    if (!base || typeof base === 'string' || typeof base.compare !== 'function') continue
    const compare = base.compare.bind(base)
    const addFormat = ajv.addFormat.bind(ajv) as (formatName: string, format: DateFormat) => unknown
    addFormat(name, {
      ...base,
      compare: (data: string, limit: string) => {
        if (limit !== 'today') return compare(data, limit)
        const day = data.slice(0, 10)
        const today = localISODate()
        if (day < today) return -1
        if (day > today) return 1
        return 0
      },
    })
  }
  return ajv
}

// $data: true — needed for formatMaximum: { $data: "1/<sibling>" } date-order rules in schemas.
// withRelativeDates runs before any schema is compiled.
export const formAjv = withRelativeDates(createAjv({ useDefaults: true, $data: true }))

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
