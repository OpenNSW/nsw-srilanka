import { http } from '@/services/http'
import { API_BASE_URL } from '@/constants'
import type { PaginatedResponse } from '@/services/types/common'
import type { SearchService } from '@opennsw/jsonforms-renderers'
import type { StaticDataOption, StaticDataResponse } from './types'

const SEARCH_LIMIT = 20

type StaticDataParams = { id: string; manifestVersion: number }

function isManifestVersion(value: unknown): value is number {
  return typeof value === 'number' && Number.isInteger(value) && value > 0
}

// params.id is the field's x-search.params.id; params.manifestVersion is the task's, added by
// withManifestVersion.
function staticDataParams(params: Record<string, unknown> | undefined): StaticDataParams {
  const id = params?.id
  if (typeof id !== 'string') {
    throw new Error('static-data search service requires x-search.params.id (a string)')
  }
  const manifestVersion = params?.manifestVersion
  if (!isManifestVersion(manifestVersion)) {
    throw new Error("static-data search service requires the task's manifest version (see withManifestVersion)")
  }
  return { id, manifestVersion }
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

// (id, manifestVersion) identifies immutable content (see internal/staticdata's Cache-Control), so a successful fetch is kept for the lifetime of the
// page: every field pointing at the same artifact, and every reopen of the same dropdown,
// reuses it instead of re-fetching. A failed fetch is evicted so the next call retries.
//
// The cached promise is deliberately never aborted: closing one dropdown (or a debounced query
// change) must not cancel a fetch that another field, or a later reopen, is also waiting on. The
// per-call `signal` the renderer hands `search()` is for the caller's own in-flight request, not
// for a shared cache entry — so it is not forwarded here.
const optionsCache = new Map<string, Promise<StaticDataOption[]>>()

function fetchOptions({ id, manifestVersion }: StaticDataParams): Promise<StaticDataOption[]> {
  const key = `${id}\0${manifestVersion}`
  const cached = optionsCache.get(key)
  if (cached) return cached

  const promise = http
    .request<StaticDataResponse>({
      url: `${API_BASE_URL}/api/v1/static-data/${encodeURIComponent(id)}`,
      params: { manifestVersion },
      attachToken: true,
    })
    .then(({ data }) => data.data.filter(isStaticDataOption))
  promise.catch(() => optionsCache.delete(key))

  optionsCache.set(key, promise)
  return promise
}

function toSearchOptions(options: StaticDataOption[]) {
  return options.map((option) => ({ id: option.const, name: option.title }))
}

// x-search.dependsOn arrives as params.parent. An empty value means no constraint.
// Options with no parents list are kept only when nothing is constraining them.
function parentParam(params: Record<string, unknown> | undefined): string | undefined {
  const parent = params?.parent
  return typeof parent === 'string' && parent.length > 0 ? parent : undefined
}

// Generic search service for `x-search.service: "static-data"` fields. A field names its artifact
// by x-search.params.id and loads it as the task's manifest version lists it, which
// withManifestVersion adds to params. This single registration backs every static-data field in
// every form.
//
// An empty query with no cursor still downloads the artifact, which is what small lists do on
// open. A typed query, or "load more", sends q, offset, and limit so the API ranks one page.
// params.parent is applied on the cached list, and on the server query before that page is cut,
// so the returned page and its total stay inside the parent scope.
export const staticDataSearchService: SearchService = {
  async search({ query, cursor, signal, params }) {
    const selected = staticDataParams(params)
    const q = query.trim()
    const offset = typeof cursor === 'number' ? cursor : 0
    const parent = parentParam(params)
    if (!q && offset === 0) {
      const options = await fetchOptions(selected)
      const scoped = parent ? options.filter((option) => option.parents?.includes(parent)) : options
      return { options: toSearchOptions(scoped) }
    }

    const { data } = await http.request<PaginatedResponse<StaticDataOption>>({
      url: `${API_BASE_URL}/api/v1/static-data/${encodeURIComponent(selected.id)}`,
      params: { manifestVersion: selected.manifestVersion, q: q || undefined, offset, limit: SEARCH_LIMIT, parent },
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
    const options = await fetchOptions(staticDataParams(params))
    const match = options.find((option) => option.const === value)
    return match ? { id: match.const, name: match.title } : undefined
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
