import { http } from '@/services/http'
import { API_BASE_URL } from '@/constants'
import type { SearchService } from '@opennsw/jsonforms-renderers'

// Shape of one entry in a `static_data` artifact served by GET /api/v1/static-data/{id} —
// see internal/staticdata (nsw-srilanka backend). Reused across every field that points at
// this service; the artifact id/version distinguish one field's data from another's.
interface StaticDataOption {
  const: string
  title: string
}

function staticDataParams(params: Record<string, unknown> | undefined): { id: string; version: string } {
  const id = params?.id
  const version = params?.version
  if (typeof id !== 'string' || typeof version !== 'string') {
    throw new Error('tnsw-static search service requires x-search.params.id and .params.version (both strings)')
  }
  return { id, version }
}

async function fetchOptions(id: string, version: string, signal?: AbortSignal): Promise<StaticDataOption[]> {
  const { data } = await http.request<StaticDataOption[]>({
    url: `${API_BASE_URL}/api/v1/static-data/${encodeURIComponent(id)}`,
    params: { version },
    attachToken: true,
    signal,
  })
  return data
}

// Generic search service for `x-search.service: "tnsw-static"` fields. One field's artifact
// (id + version) is selected entirely via x-search.params, so this single registration backs
// every static-data field in every form.
export const tnswStaticSearchService: SearchService = {
  async search({ query, signal, params }) {
    const { id, version } = staticDataParams(params)
    const options = await fetchOptions(id, version, signal)

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