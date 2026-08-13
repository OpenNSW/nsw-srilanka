import { http } from '@/services/http'
import { API_BASE_URL } from '@/constants'
import type { SearchService } from '@opennsw/jsonforms-renderers'

const LIMIT = 20

interface CompanySummary {
  id: string
  name: string
  hasCha: boolean
}

interface CompanyListResult {
  items: CompanySummary[]
  total: number
}

export const chaSearchService: SearchService = {
  async search({ query, cursor, signal }) {
    const offset = (cursor as number) ?? 0
    const { data } = await http.request<CompanyListResult>({
      url: `${API_BASE_URL}/api/v1/companies`,
      params: { has_cha: true, name: query || undefined, offset, limit: LIMIT },
      attachToken: true,
      signal,
    })
    const nextOffset = offset + data.items.length
    return {
      options: data.items.map((c) => ({ id: c.id, name: c.name })),
      nextCursor: nextOffset < data.total ? nextOffset : undefined,
    }
  },
}
