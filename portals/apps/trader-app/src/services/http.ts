import { userManager } from '../oidcUserManager'

interface RequestConfig {
  url: string
  method?: string
  headers?: Record<string, string>
  params?: Record<string, string | number | boolean | undefined | null>
  data?: unknown
  attachToken?: boolean
  signal?: AbortSignal
}

export const http = {
  request: async (config: RequestConfig) => {
    let url = config.url
    if (config.params) {
      const searchParams = new URLSearchParams()
      for (const [key, value] of Object.entries(config.params)) {
        if (value !== undefined && value !== null) {
          searchParams.append(key, String(value))
        }
      }
      const queryString = searchParams.toString()
      if (queryString) {
        url += (url.includes('?') ? '&' : '?') + queryString
      }
    }

    const headers = { ...config.headers }

    if (config.attachToken) {
      const user = await userManager.getUser()
      if (user?.access_token) {
        headers['Authorization'] = `Bearer ${user.access_token}`
      }
    }

    if (config.data && !headers['Content-Type']) {
      headers['Content-Type'] = 'application/json'
    }

    const response = await fetch(url, {
      method: config.method || 'GET',
      headers,
      body: config.data ? JSON.stringify(config.data) : undefined,
      signal: config.signal,
    })

    if (!response.ok) {
      throw new Error(`HTTP error! status: ${response.status}`)
    }

    const contentType = response.headers.get('content-type')
    const data = (
      contentType && contentType.includes('application/json') ? await response.json() : await response.text()
    ) as unknown

    return { data }
  },
}
