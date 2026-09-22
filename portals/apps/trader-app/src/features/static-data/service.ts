import { http } from '@/services/http'
import { API_BASE_URL } from '@/constants'
import type { PaginatedResponse } from '@/services/types/common'
import type { SearchService } from '@opennsw/jsonforms-renderers'
import type { StaticDataOption, StaticDataResponse } from './types'

const SEARCH_LIMIT = 20

function staticDataParams(params: Record<string, unknown> | undefined): { id: string; version: string } {
  const id = params?.id
  const version = params?.version
  if (typeof id !== 'string' || typeof version !== 'string') {
    throw new Error('static-data search service requires x-search.params.id and .params.version (both strings)')
  }
  return { id, version }
}

// The artifact source isn't type-checked against this contract, so a malformed entry (a bare
// string, a missing/non-string field) is dropped here rather than being offered as an option.
function isStaticDataOption(value: unknown): value is StaticDataOption {
  return (
    typeof value === 'object' &&
    value !== null &&
    typeof (value as StaticDataOption).const === 'string' &&
    typeof (value as StaticDataOption).title === 'string'
  )
}

// (id, version) identifies immutable content (see internal/staticdata's Cache-Control), so a
// successful fetch is kept for the lifetime of the page: every field pointing at the same
// artifact, and every reopen of the same dropdown, reuses it instead of re-fetching. A failed
// fetch is evicted so the next call retries.
//
// The cached promise is deliberately never aborted: closing one dropdown (or a debounced query
// change) must not cancel a fetch that another field, or a later reopen, is also waiting on. The
// per-call `signal` the renderer hands `search()` is for the caller's own in-flight request, not
// for a shared cache entry — so it is not forwarded here.
const optionsCache = new Map<string, Promise<StaticDataOption[]>>()

function fetchOptions(id: string, version: string): Promise<StaticDataOption[]> {
  const key = `${id}\0${version}`
  const cached = optionsCache.get(key)
  if (cached) return cached

  const promise = http
    .request<StaticDataResponse>({
      url: `${API_BASE_URL}/api/v1/static-data/${encodeURIComponent(id)}`,
      params: { version },
      attachToken: true,
    })
    .then(({ data }) => data.data.filter(isStaticDataOption))
  promise.catch(() => optionsCache.delete(key))

  optionsCache.set(key, promise)
  return promise
}

// Titles are not unique (several ports are named HAMPTON). Show the code beside
// the title so each row can be told apart. Identical code and title stay as one label.
function toSearchOptions(options: StaticDataOption[]) {
  return options.map((option) => ({
    id: option.const,
    name: option.const === option.title ? option.title : `${option.const}-${option.title}`,
  }))
}

// Generic search service for `x-search.service: "static-data"` fields. One field's artifact
// (id + version) is selected entirely via x-search.params, so this single registration backs
// every static-data field in every form.
//
// An empty query with no cursor still downloads the artifact, which is what small lists do on
// open. A typed query, or "load more", sends q, offset, and limit so the API ranks one page.
export const staticDataSearchService: SearchService = {
  async search({ query, cursor, signal, params }) {
    const { id, version } = staticDataParams(params)
    const q = query.trim()
    const offset = typeof cursor === 'number' ? cursor : 0
    if (!q && offset === 0) {
      const options = await fetchOptions(id, version)
      return { options: toSearchOptions(options) }
    }

    const { data } = await http.request<PaginatedResponse<StaticDataOption>>({
      url: `${API_BASE_URL}/api/v1/static-data/${encodeURIComponent(id)}`,
      params: { version, q: q || undefined, offset, limit: SEARCH_LIMIT },
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
    const { id, version } = staticDataParams(params)
    const options = await fetchOptions(id, version)
    const match = options.find((option) => option.const === value)
    return match ? { id: match.const, name: match.title } : undefined
  },
}
