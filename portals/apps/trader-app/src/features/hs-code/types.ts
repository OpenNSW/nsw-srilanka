import type { PaginatedResponse } from '@/services/types/common'

export interface HSCode {
  id: string
  hsCode: string
  description: string
  category: string
}

export interface HSCodeQueryParams {
  hsCodeStartsWith?: string
  limit?: number
  offset?: number
}

export type HSCodeListResult = PaginatedResponse<HSCode>
