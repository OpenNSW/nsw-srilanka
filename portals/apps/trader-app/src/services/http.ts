import { userManager } from '@/oidcUserManager'

interface RequestConfig {
  url: string
  method?: string
  headers?: Record<string, string>
  params?: Record<string, string | number | boolean | undefined | null>
  data?: unknown
  attachToken?: boolean
  signal?: AbortSignal
}

export class HttpError extends Error {
  readonly status: number
  readonly statusText: string
  readonly body: unknown

  constructor(status: number, statusText: string, body: unknown) {
    super(`HTTP error! status: ${status} ${statusText}`)
    this.name = 'HttpError'
    this.status = status
    this.statusText = statusText
    this.body = body
  }
}

const inFlightRequests = new Map<string, Promise<{ data: unknown }>>()

function isPlainObject(value: unknown): boolean {
  if (typeof value !== 'object' || value === null) return false
  const proto = Object.getPrototypeOf(value) as unknown
  return proto === null || proto === Object.prototype
}

export const http = {
  request: async <T = unknown>(config: RequestConfig): Promise<{ data: T }> => {
    let url = config.url
    if (config.params) {
      const searchParams = new URLSearchParams()
      Object.entries(config.params)
        .filter(([, value]) => value !== undefined && value !== null)
        .sort(([a], [b]) => a.localeCompare(b))
        .forEach(([key, value]) => searchParams.append(key, String(value)))
      const queryString = searchParams.toString()
      if (queryString) {
        url += (url.includes('?') ? '&' : '?') + queryString
      }
    }

    const isGet = !config.method || config.method.toUpperCase() === 'GET'
    const isCacheable = isGet && !config.signal

    if (isCacheable && inFlightRequests.has(url)) {
      return inFlightRequests.get(url)! as Promise<{ data: T }>
    }

    const promise = (async (): Promise<{ data: T }> => {
      const headers: Record<string, string> = { ...config.headers }

      if (config.attachToken) {
        const user = await userManager.getUser()
        if (user?.access_token) {
          headers['Authorization'] = `Bearer ${user.access_token}`
        }
      }

      const serializableBody = isPlainObject(config.data)
      if (serializableBody && !headers['Content-Type']) {
        headers['Content-Type'] = 'application/json'
      }

      const response = await fetch(url, {
        method: config.method || 'GET',
        headers,
        body:
          config.data !== undefined
            ? serializableBody
              ? JSON.stringify(config.data)
              : (config.data as BodyInit)
            : undefined,
        signal: config.signal,
      })

      if (!response.ok) {
        const errorText = await response.text().catch(() => '')
        let body: unknown = errorText
        try {
          body = JSON.parse(errorText) as unknown
        } catch {
          // errorText is not valid JSON — use raw string as body
        }
        throw new HttpError(response.status, response.statusText, body)
      }

      const text = await response.text()
      const contentType = response.headers.get('content-type')
      let data: unknown = text
      if (contentType?.includes('application/json') && text) {
        try {
          data = JSON.parse(text) as unknown
        } catch (e) {
          throw new Error(`Failed to parse JSON response body: ${e instanceof Error ? e.message : String(e)}`, {
            cause: e,
          })
        }
      }

      return { data: data as T }
    })()

    if (isCacheable) {
      inFlightRequests.set(url, promise)
      void promise.finally(() => inFlightRequests.delete(url))
    }

    return promise
  },
}
