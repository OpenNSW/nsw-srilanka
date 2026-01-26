export interface HSCode {
  id: string
  code: string
  description: string
  parentCode: string | null
  level: number
}

export interface HSCodeQueryParams {
  hs_code?: string
  limit?: number
  offset?: number
}