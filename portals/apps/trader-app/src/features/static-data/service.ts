import { http } from '@/services/http'
import { API_BASE_URL } from '@/constants'
import type { SearchService } from '@opennsw/jsonforms-renderers'
import type { StaticDataOption, StaticDataResponse } from './types'

function staticDataParams(params: Record<string, unknown> | undefined): { id: string; version: string } {
  const id = params?.id
  const version = params?.version
  if (typeof id !== 'string' || typeof version !== 'string') {
    throw new Error('static-data search service requires x-search.params.id and .params.version (both strings)')
  }
  return { id, version }
}

// The artifact source isn't type-checked against this contract, so a malformed entry (a bare
// string, a missing/non-string field) is dropped here rather than reaching `.toLowerCase()` below.
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

// Generic search service for `x-search.service: "static-data"` fields. One field's artifact
// (id + version) is selected entirely via x-search.params, so this single registration backs
// every static-data field in every form.
export const staticDataSearchService: SearchService = {
  async search({ query, params }) {
    const { id, version } = staticDataParams(params)
    const options = await fetchOptions(id, version)

    const q = query.trim().toLowerCase()
    const filtered = q ? options.filter((option) => option.title.toLowerCase().includes(q)) : options

    return { options: filtered.map((option) => ({ id: option.const, name: option.title })) }
  },

  async resolve(value, params) {
    const { id, version } = staticDataParams(params)
    const options = await fetchOptions(id, version)
    const match = options.find((option) => option.const === value)
    return match ? { id: match.const, name: match.title } : undefined
  },
}
