import { getEnv } from '../runtimeConfig'

const API_BASE_URL = getEnv('VITE_API_BASE_URL', 'http://localhost:8080/api/v1')!

export type QueryParams = Record<string, string | number | undefined>
export type AccessTokenProvider = () => Promise<string | null | undefined>

export interface ApiClient {
  get<T>(endpoint: string, params?: QueryParams): Promise<T>
  post<T, R>(endpoint: string, body: T): Promise<R>
}


export type ErrorResponse = {
  code: string
  message: string
  details: unknown
}

export type ApiResponse<T> = {
  success: boolean
  data: T
  error?: ErrorResponse
}

export type PaginatedResponse<T> = {
  data: T[]
  total: number
  page: number
  pageSize: number
  totalPages: number
}

function buildQueryString(params: QueryParams): string {
  const entries = Object.entries(params)
    .filter(([, value]) => value !== undefined)
    .sort(([left], [right]) => left.localeCompare(right))

  const searchParams = new URLSearchParams()
  entries.forEach(([key, value]) => {
    searchParams.append(key, String(value))
  })

  return searchParams.toString()
}

function buildTokenFingerprint(token: string | null): string {
  if (!token) {
    return 'anonymous'
  }
  return `${token.length}:${token.slice(-16)}`
}

function buildRequestKey(endpoint: string, params: QueryParams = {}, token: string | null): string {
  const queryString = buildQueryString(params)
  const tokenFingerprint = buildTokenFingerprint(token)
  return `GET:${tokenFingerprint}:${endpoint}?${queryString}`
}

async function buildHeaders(token?: string | null): Promise<HeadersInit> {
  const headers: Record<string, string> = {
    'Content-Type': 'application/json',
  }

  if (token) {
    headers.Authorization = `Bearer ${token}`
  }

  return headers
}

export async function apiGet<T>(
  endpoint: string,
  params: QueryParams = {},
  token?: string | null
): Promise<T> {
  const queryString = buildQueryString(params)
  const url = `${API_BASE_URL}${endpoint}${queryString ? `?${queryString}` : ''}`

  const response = await fetch(url, {
    headers: await buildHeaders(token),
  })
  if (!response.ok) {
    throw new Error(`API error: ${response.status} ${response.statusText}`)
  }
  return (await response.json()) as T
}

export async function apiPost<T, R>(
  endpoint: string,
  body: T,
  token?: string | null
): Promise<R> {
  const url = `${API_BASE_URL}${endpoint}`

  const response = await fetch(url, {
    method: 'POST',
    headers: await buildHeaders(token),
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

export function createApiClient(getAccessToken?: AccessTokenProvider): ApiClient {
  const inFlightGetRequests = new Map<string, Promise<unknown>>()

  return {
    async get<T>(endpoint: string, params: QueryParams = {}): Promise<T> {
      const token = getAccessToken ? (await getAccessToken()) ?? null : null
      const requestKey = buildRequestKey(endpoint, params, token)

      const existingRequest = inFlightGetRequests.get(requestKey)
      if (existingRequest) {
        return existingRequest as Promise<T>
      }

      const requestPromise = (async () => {
        return apiGet<T>(endpoint, params, token)
      })()

      inFlightGetRequests.set(requestKey, requestPromise)

      try {
        return await requestPromise
      } finally {
        inFlightGetRequests.delete(requestKey)
      }
    },
    async post<T, R>(endpoint: string, body: T): Promise<R> {
      const token = getAccessToken ? await getAccessToken() : null
      return apiPost<T, R>(endpoint, body, token)
    },
  }
}

export const defaultApiClient = createApiClient()
