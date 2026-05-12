import { API_BASE_URL, API_PATH_PREFIX } from '../constants'

export type QueryParams = Record<string, string | number | undefined>
export type AccessTokenProvider = () => Promise<string | null | undefined>

export interface ApiClient {
  get<T>(endpoint: string, params?: QueryParams): Promise<T>
  post<T, R>(endpoint: string, body: T): Promise<R>
  put<T, R>(endpoint: string, body: T): Promise<R>
  getAuthHeaders(includeJsonContentType?: boolean): Promise<HeadersInit>
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

function buildQueryParams(params: QueryParams): URLSearchParams {
  const entries = Object.entries(params)
    .filter(([, value]) => value !== undefined)
    .sort(([left], [right]) => left.localeCompare(right))

  const searchParams = new URLSearchParams()
  entries.forEach(([key, value]) => {
    searchParams.append(key, String(value))
  })

  return searchParams
}

function buildTokenFingerprint(token: string | null): string {
  if (!token) {
    return 'anonymous'
  }
  return `${token.length}:${token.slice(-16)}`
}

function buildRequestKey(endpoint: string, params: QueryParams = {}, token: string | null): string {
  const queryString = buildQueryParams(params).toString()
  const tokenFingerprint = buildTokenFingerprint(token)
  return `GET:${tokenFingerprint}:${endpoint}?${queryString}`
}

async function buildHeaders(token?: string | null, includeJsonContentType = true): Promise<HeadersInit> {
  const headers: Record<string, string> = {}

  if (includeJsonContentType) {
    headers['Content-Type'] = 'application/json'
  }

  if (token) {
    headers.Authorization = `Bearer ${token}`
  }

  return headers
}

export async function apiGet<T>(endpoint: string, params: QueryParams = {}, token?: string | null): Promise<T> {
  const url = new URL(API_PATH_PREFIX + endpoint, API_BASE_URL)
  const queryParams = buildQueryParams(params)
  if (queryParams.toString()) {
    url.search = queryParams.toString()
  }
  const finalUrl = url.toString()
  const response = await fetch(finalUrl, {
    headers: await buildHeaders(token),
  })
  if (!response.ok) {
    throw new Error(`API error: ${response.status} ${response.statusText}`)
  }
  return (await response.json()) as T
}

export async function apiPost<T, R>(endpoint: string, body: T, token?: string | null): Promise<R> {
  const url = new URL(API_PATH_PREFIX + endpoint, API_BASE_URL).toString()

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

export async function apiPut<T, R>(endpoint: string, body: T, token?: string | null): Promise<R> {
  const url = new URL(API_PATH_PREFIX + endpoint, API_BASE_URL).toString()

  const response = await fetch(url, {
    method: 'PUT',
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
      const token = getAccessToken ? ((await getAccessToken()) ?? null) : null
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
    async put<T, R>(endpoint: string, body: T): Promise<R> {
      const token = getAccessToken ? await getAccessToken() : null
      return apiPut<T, R>(endpoint, body, token)
    },
    async getAuthHeaders(includeJsonContentType = true): Promise<HeadersInit> {
      const token = getAccessToken ? ((await getAccessToken()) ?? null) : null
      return buildHeaders(token, includeJsonContentType)
    },
  }
}

export const defaultApiClient = createApiClient()
