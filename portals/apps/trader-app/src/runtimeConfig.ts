type RuntimeConfigValue = string | undefined

type RuntimeConfigMap = Record<string, RuntimeConfigValue>

declare global {
  interface Window {
    __APP_CONFIG__?: RuntimeConfigMap
  }
}

function resolveRuntimeConfig(): RuntimeConfigMap {
  if (typeof window === 'undefined') {
    return {}
  }

  return window.__APP_CONFIG__ ?? {}
}

export function getEnv(name: string): string | undefined
export function getEnv(name: string, fallback: string): string
export function getEnv(name: string, fallback?: string): string | undefined {
  const runtimeValue = resolveRuntimeConfig()[name]
  if (runtimeValue && runtimeValue.trim() !== '') {
    return runtimeValue
  }

  return fallback
}

export function getRequiredEnv(name: string): string {
  const value = getEnv(name)
  if (!value || value.trim() === '') {
    throw new Error(`Missing required environment variable: ${name}`)
  }

  return value
}

export function getBooleanEnv(name: string, fallback = false): boolean {
  const value = getEnv(name)
  if (!value) {
    return fallback
  }

  return value.toLowerCase() === 'true'
}
