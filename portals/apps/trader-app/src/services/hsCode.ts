import { apiGet, USE_MOCK, type PaginatedResponse } from './api'
import type { HSCode, HSCodeQueryParams } from './types/hsCode'
import { mockHSCodes } from './mocks/hsCodeData'

function getMockHSCodes(params: HSCodeQueryParams): PaginatedResponse<HSCode> {
  const { hs_code, limit = 10, offset = 0 } = params

  let filtered = mockHSCodes

  if (hs_code) {
    filtered = mockHSCodes.filter((item) => item.code.startsWith(hs_code))
  }

  const paginated = filtered.slice(offset, offset + limit)

  return {
    data: paginated,
    total: filtered.length,
    limit,
    offset,
  }
}

export async function getHSCodes(
  params: HSCodeQueryParams = {}
): Promise<PaginatedResponse<HSCode>> {
  if (USE_MOCK) {
    // Simulate network delay
    await new Promise((resolve) => setTimeout(resolve, 300))
    return getMockHSCodes(params)
  }

  return apiGet<PaginatedResponse<HSCode>>('/hs-codes', {
    hs_code: params.hs_code,
    limit: params.limit,
    offset: params.offset,
  })
}