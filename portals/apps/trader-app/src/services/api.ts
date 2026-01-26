const API_BASE_URL = import.meta.env.VITE_API_BASE_URL || 'http://localhost:3000/api'
const USE_MOCK = import.meta.env.VITE_USE_MOCK !== 'false'

export interface PaginatedResponse<T> {
  data: T[]
  total: number
  limit: number
  offset: number
}

export interface QueryParams {
  [key:string]: string | number | undefined
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

  const response = await fetch(url)
  if (!response.ok) {
    throw new Error(`API error: ${response.status} ${response.statusText}`)
  }
  return response.json()
}

export async function apiPost<T, R>(
  endpoint: string,
  body: T
): Promise<R> {
  const url = `${API_BASE_URL}${endpoint}`

  const response = await fetch(url, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
    },
    body: JSON.stringify(body),
  })
  if (!response.ok) {
    throw new Error(`API error: ${response.status} ${response.statusText}`)
  }
  return response.json()
}

export { USE_MOCK }