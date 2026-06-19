import { http } from '@/services/http'
import { API_BASE_URL } from '@/constants'
import type { HSCodeListResult, HSCodeQueryParams } from './types'

export async function getHSCodes(params: HSCodeQueryParams = {}): Promise<HSCodeListResult> {
  const queryParams: Record<string, string | number> = {}
  if (params.hsCodeStartsWith) queryParams.hsCodeStartsWith = params.hsCodeStartsWith
  if (params.limit !== undefined) queryParams.limit = params.limit
  if (params.offset !== undefined) queryParams.offset = params.offset

  const { data } = await http.request<HSCodeListResult>({
    url: `${API_BASE_URL}/api/v1/hscodes`,
    params: queryParams,
    attachToken: true,
  })
  return data
}
