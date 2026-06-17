import { http } from './http'
import { API_BASE_URL, API_PATH_PREFIX } from '../constants'
import type { HSCodeListResult, HSCodeQueryParams } from './types/hsCode'

const BASE = `${API_BASE_URL}${API_PATH_PREFIX}`

export async function getHSCodes(params: HSCodeQueryParams = {}): Promise<HSCodeListResult> {
  const queryParams: Record<string, string | number> = {}
  if (params.hsCodeStartsWith) queryParams.hsCodeStartsWith = params.hsCodeStartsWith
  if (params.limit !== undefined) queryParams.limit = params.limit
  if (params.offset !== undefined) queryParams.offset = params.offset

  const { data } = await http.request({
    url: `${BASE}/hscodes`,
    params: queryParams,
    attachToken: true,
  })
  return data as HSCodeListResult
}
