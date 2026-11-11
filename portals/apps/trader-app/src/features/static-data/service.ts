import { http } from '@/services/http'
import { API_BASE_URL } from '@/constants'
import type { PaginatedResponse } from '@/services/types/common'
import type { SearchOption, SearchService } from '@opennsw/jsonforms-renderers'
import type { StaticDataResponse, StaticDataRow } from './types'

const SEARCH_LIMIT = 20

// The row key a field stores when its x-search.params names no valueKey.
const DEFAULT_VALUE_KEY = 'const'

interface StaticDataParams {
  id: string
  version: string
  valueKey: string
  // undefined lets the API use its default keys.
  searchKeys?: string[]
}

function isStringList(value: unknown): value is string[] {
  return Array.isArray(value) && value.every((item) => typeof item === 'string' && item.length > 0)
}

function staticDataParams(params: Record<string, unknown> | undefined): StaticDataParams {
  const { id, version, valueKey = DEFAULT_VALUE_KEY, searchKeys } = params ?? {}
  if (typeof id !== 'string' || typeof version !== 'string') {
    throw new Error('static-data search service requires x-search.params.id and .params.version (both strings)')
  }
  if (typeof valueKey !== 'string' || valueKey.length === 0) {
    throw new Error('static-data search service requires x-search.params.valueKey to be a non-empty string')
  }
  if (searchKeys !== undefined && !isStringList(searchKeys)) {
    throw new Error('static-data search service requires x-search.params.searchKeys to be a list of key names')
  }
  return { id, version, valueKey, searchKeys }
}

// The artifact source isn't type-checked against this contract, so an entry that isn't an
// object (a bare string, an array, null) is dropped here rather than offered as an option.
function isStaticDataRow(value: unknown): value is StaticDataRow {
  return typeof value === 'object' && value !== null && !Array.isArray(value)
}

// (id, version) identifies immutable content (see internal/staticdata's Cache-Control), so a
// successful fetch is kept for the lifetime of the page: every field pointing at the same
// artifact, and every reopen of the same dropdown, reuses it instead of re-fetching. A failed
// fetch is evicted so the next call retries. Fields can store different keys of the same
// artifact, so the cache keeps every row and each field picks its own.
//
// The cached promise is deliberately never aborted: closing one dropdown (or a debounced query
// change) must not cancel a fetch that another field, or a later reopen, is also waiting on. The
// per-call `signal` the renderer hands `search()` is for the caller's own in-flight request, not
// for a shared cache entry — so it is not forwarded here.
const rowsCache = new Map<string, Promise<StaticDataRow[]>>()

function fetchRows(id: string, version: string): Promise<StaticDataRow[]> {
  const key = `${id}\0${version}`
  const cached = rowsCache.get(key)
  if (cached) return cached

  const promise = http
    .request<StaticDataResponse>({
      url: `${API_BASE_URL}/api/v1/static-data/${encodeURIComponent(id)}`,
      params: { version },
      attachToken: true,
    })
    .then(({ data }) => data.data.filter(isStaticDataRow))
  promise.catch(() => rowsCache.delete(key))

  rowsCache.set(key, promise)
  return promise
}

// A row as a search option: the row's own keys, so a displayTemplate can use any of them,
// plus the id and name every SearchOption carries. id is the stored value; name is the row's
// title when it has one, else the value. Both are set last so they keep that meaning; {id}
// and {name} are the legacy placeholders.
function toSearchOption(row: StaticDataRow, value: string): SearchOption {
  return { ...row, id: value, name: typeof row.title === 'string' ? row.title : value }
}

// Rows without a string value under valueKey can't be stored, so they aren't offered.
function toSearchOptions(rows: StaticDataRow[], valueKey: string): SearchOption[] {
  return rows.flatMap((row) => {
    const value = row[valueKey]
    return typeof value === 'string' ? [toSearchOption(row, value)] : []
  })
}

// x-search.dependsOn arrives as params.parent. An empty value means no constraint.
// Rows with no parents list are kept only when nothing is constraining them.
function parentParam(params: Record<string, unknown> | undefined): string | undefined {
  const parent = params?.parent
  return typeof parent === 'string' && parent.length > 0 ? parent : undefined
}

// Generic search service for `x-search.service: "static-data"` fields. One field's artifact
// (id + version) is selected entirely via x-search.params, so this single registration backs
// every static-data field in every form. params.valueKey names the row key the field stores
// (default "const"), and params.searchKeys the row keys a typed search matches, in priority
// order (default: the API's, title then const).
//
// An empty query with no cursor still downloads the artifact, which is what small lists do on
// open. A typed query, or "load more", sends q, offset, limit, and searchKeys so the API ranks
// one page. params.parent is applied on the cached list, and on the server query before that
// page is cut, so the returned page and its total stay inside the parent scope.
export const staticDataSearchService: SearchService = {
  async search({ query, cursor, signal, params }) {
    const { id, version, valueKey, searchKeys } = staticDataParams(params)
    const q = query.trim()
    const offset = typeof cursor === 'number' ? cursor : 0
    const parent = parentParam(params)
    if (!q && offset === 0) {
      const rows = await fetchRows(id, version)
      const scoped = parent ? rows.filter((row) => row.parents?.includes(parent)) : rows
      return { options: toSearchOptions(scoped, valueKey) }
    }

    const { data } = await http.request<PaginatedResponse<StaticDataRow>>({
      url: `${API_BASE_URL}/api/v1/static-data/${encodeURIComponent(id)}`,
      params: { version, q: q || undefined, offset, limit: SEARCH_LIMIT, parent, searchKeys: searchKeys?.join(',') },
      attachToken: true,
      signal,
    })
    // The cursor counts the rows the API returned, including any that aren't offered.
    const nextOffset = offset + data.items.length
    return {
      options: toSearchOptions(data.items, valueKey),
      nextCursor: nextOffset < data.total ? nextOffset : undefined,
    }
  },

  async resolve(value, params) {
    const { id, version, valueKey } = staticDataParams(params)
    const rows = await fetchRows(id, version)
    const match = rows.find((row) => row[valueKey] === value)
    return match ? toSearchOption(match, value) : undefined
  },
}
