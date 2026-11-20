import { http } from '@/services/http'
import { API_BASE_URL } from '@/constants'
import type { PaginatedResponse } from '@/services/types/common'
import type { SearchOption, SearchService } from '@opennsw/jsonforms-renderers'
import type { StaticDataResponse, StaticDataRow } from './types'

const SEARCH_LIMIT = 20

interface StaticDataParams {
  id: string
  manifestVersion: number
  // undefined lets the API use its default keys.
  searchKeys?: string[]
}

// A key name as the API accepts it (core/staticdata handler.go): the characters a
// displayTemplate placeholder can hold. A comma or a space would change what the API reads from
// the comma-joined list sent on a typed search.
const SEARCH_KEY_PATTERN = /^[A-Za-z0-9_]+$/

function isSearchKeyList(value: unknown): value is string[] {
  return Array.isArray(value) && value.every((item) => typeof item === 'string' && SEARCH_KEY_PATTERN.test(item))
}

function isManifestVersion(value: unknown): value is number {
  return typeof value === 'number' && Number.isInteger(value) && value > 0
}

// params.id and params.searchKeys are the field's x-search.params; params.manifestVersion is the
// task's, added by withManifestVersion.
function staticDataParams(params: Record<string, unknown> | undefined): StaticDataParams {
  const { id, manifestVersion, searchKeys } = params ?? {}
  if (typeof id !== 'string') {
    throw new Error('static-data search service requires x-search.params.id (a string)')
  }
  if (!isManifestVersion(manifestVersion)) {
    throw new Error("static-data search service requires the task's manifest version (see withManifestVersion)")
  }
  if (searchKeys !== undefined && !isSearchKeyList(searchKeys)) {
    throw new Error(
      'static-data search service requires x-search.params.searchKeys to be a list of key names (letters, digits and underscores)',
    )
  }
  return { id, manifestVersion, searchKeys }
}

function isNonEmptyString(value: unknown): value is string {
  return typeof value === 'string' && value.length > 0
}

// The artifact source isn't type-checked against this contract, so a malformed entry (a bare
// string, a missing, non-string or empty const or title) is dropped here rather than being
// offered as an option. The API rejects the same rows on a typed search.
function isStaticDataRow(value: unknown): value is StaticDataRow {
  return (
    typeof value === 'object' &&
    value !== null &&
    isNonEmptyString((value as StaticDataRow).const) &&
    isNonEmptyString((value as StaticDataRow).title)
  )
}

// (id, manifestVersion) identifies immutable content (see core/staticdata's Cache-Control), so a successful fetch is kept for the lifetime of the
// page: every field pointing at the same artifact, and every reopen of the same dropdown,
// reuses it instead of re-fetching. A failed fetch is evicted so the next call retries.
//
// The cached promise is deliberately never aborted: closing one dropdown (or a debounced query
// change) must not cancel a fetch that another field, or a later reopen, is also waiting on. The
// per-call `signal` the renderer hands `search()` is for the caller's own in-flight request, not
// for a shared cache entry — so it is not forwarded here.
const rowsCache = new Map<string, Promise<StaticDataRow[]>>()

function fetchRows(id: string, manifestVersion: number): Promise<StaticDataRow[]> {
  const key = `${id}\0${manifestVersion}`
  const cached = rowsCache.get(key)
  if (cached) return cached

  const promise = http
    .request<StaticDataResponse>({
      url: `${API_BASE_URL}/api/v1/static-data/${encodeURIComponent(id)}`,
      params: { manifestVersion },
      attachToken: true,
    })
    .then(({ data }) => data.data.filter(isStaticDataRow))
  promise.catch(() => rowsCache.delete(key))

  rowsCache.set(key, promise)
  return promise
}

// A row as a search option: the row's own keys, so a displayTemplate can use {const}, {title}
// and any other key, plus the id and name every SearchOption carries. id and name are set last,
// so they always mean const and title; {id} and {name} are the legacy placeholders.
function toSearchOption(row: StaticDataRow): SearchOption {
  return { ...row, id: row.const, name: row.title }
}

function toSearchOptions(rows: StaticDataRow[]): SearchOption[] {
  return rows.map(toSearchOption)
}

// x-search.dependsOn arrives as params.parent. An empty value means no constraint.
// Rows with no parents list are kept only when nothing is constraining them.
function parentParam(params: Record<string, unknown> | undefined): string | undefined {
  const parent = params?.parent
  return typeof parent === 'string' && parent.length > 0 ? parent : undefined
}

// Generic search service for `x-search.service: "static-data"` fields. A field names its artifact
// by x-search.params.id and loads it as the task's manifest version lists it, which
// withManifestVersion adds to params. This single registration backs every static-data field in
// every form. params.searchKeys names the row keys a typed search matches, in priority order
// (default: the API's, title then const).
//
// An empty query with no cursor still downloads the artifact, which is what small lists do on
// open. A typed query, or "load more", sends q, offset, limit, and searchKeys so the API ranks
// one page. params.parent is applied on the cached list, and on the server query before that
// page is cut, so the returned page and its total stay inside the parent scope.
export const staticDataSearchService: SearchService = {
  async search({ query, cursor, signal, params }) {
    const { id, manifestVersion, searchKeys } = staticDataParams(params)
    const q = query.trim()
    const offset = typeof cursor === 'number' ? cursor : 0
    const parent = parentParam(params)
    if (!q && offset === 0) {
      const rows = await fetchRows(id, manifestVersion)
      const scoped = parent ? rows.filter((row) => row.parents?.includes(parent)) : rows
      return { options: toSearchOptions(scoped) }
    }

    const { data } = await http.request<PaginatedResponse<StaticDataRow>>({
      url: `${API_BASE_URL}/api/v1/static-data/${encodeURIComponent(id)}`,
      params: {
        manifestVersion,
        q: q || undefined,
        offset,
        limit: SEARCH_LIMIT,
        parent,
        searchKeys: searchKeys?.join(','),
      },
      attachToken: true,
      signal,
    })
    const nextOffset = offset + data.items.length
    return {
      options: toSearchOptions(data.items),
      nextCursor: nextOffset < data.total ? nextOffset : undefined,
    }
  },

  async resolve(value, params) {
    const { id, manifestVersion } = staticDataParams(params)
    const rows = await fetchRows(id, manifestVersion)
    const match = rows.find((row) => row.const === value)
    return match ? toSearchOption(match) : undefined
  },
}

// withManifestVersion returns service with manifestVersion added to every call's params, so a
// task's forms load static data as the task's own manifest lists it.
export function withManifestVersion(service: SearchService, manifestVersion: number): SearchService {
  const bind = (params: Record<string, unknown> | undefined) => ({ ...params, manifestVersion })
  const bound: SearchService = { search: (args) => service.search({ ...args, params: bind(args.params) }) }
  if (service.resolve) {
    bound.resolve = async (value, params) => service.resolve?.(value, bind(params))
  }
  return bound
}
