const API_BASE_URL = import.meta.env.VITE_API_BASE_URL || 'http://localhost:8080/api/v1'

// TODO: Should remove this after implementing proper authentication, and use the auth token instead of a hardcoded trader ID
const TRADER_ID = 'TRADER-001'

export interface PaginatedResponse<T> {
  items: T[]
  totalCount: number
  limit: number
  offset: number
}

export type ErrorResponse = {
  code : string
  message: string
  details: unknown
}

export type ApiResponse<T> = {
  success: boolean
  data: T
  error?: ErrorResponse
}

export interface QueryParams {
  [key: string]: string | number | undefined
}

function buildQueryString(params: QueryParams): string {
  const searchParams = new URLSearchParams()
  Object.entries(params).forEach(([key, value]) => {
    if (value !== undefined) {
      searchParams.append(key, String(value))
    }
  })
  return searchParams.toString()
}

export async function apiGet<T>(
  endpoint: string,
  params: QueryParams = {}
): Promise<T> {
  const queryString = buildQueryString(params)
  const url = `${API_BASE_URL}${endpoint}${queryString ? `?${queryString}` : ''}`

  const response = await fetch(url, {
    headers: {
      'Authorization': TRADER_ID,
      'Content-Type': 'application/json',
    },
  })
  if (!response.ok) {
    throw new Error(`API error: ${response.status} ${response.statusText}`)
  }
  return (await response.json()) as T
}

export async function apiPost<T, R>(
  endpoint: string,
  body: T
): Promise<R> {
  const url = `${API_BASE_URL}${endpoint}`

  const response = await fetch(url, {
    method: 'POST',
    headers: {
      'Authorization': TRADER_ID,
      'Content-Type': 'application/json',
    },
    body: JSON.stringify(body),
  })

  if (!response.ok) {
    const errorText = await response.text()
    console.error(`API error ${response.status}: ${errorText}`)
    throw new Error(`API error: ${response.status} ${response.statusText} - ${errorText}`)
  }

  const text = await response.text()
  if (!text) {
    throw new Error('API returned empty response')
  }

  try {
    return JSON.parse(text) as R
  } catch (e) {
    console.error('Failed to parse API response', text)
    throw new Error(`Failed to parse API response: ${e instanceof Error ? e.message : String(e)}`)
  }
}
