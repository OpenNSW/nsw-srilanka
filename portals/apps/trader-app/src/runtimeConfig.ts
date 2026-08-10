type RuntimeConfigValue = string | undefined

type RuntimeConfigMap = Record<string, RuntimeConfigValue>

export interface IdpRoleGroupConfig {
  traderGroupName: string
  chaGroupName: string
}

const DEFAULT_TRADER_GROUP_NAME = 'Traders'
const DEFAULT_CHA_GROUP_NAME = 'CHA'

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

  const buildValue = (import.meta.env as Record<string, string | undefined>)[name]
  if (buildValue && buildValue.trim() !== '') {
    return buildValue
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

// Set by oidc-client-ts itself; overriding one corrupts the authorization request.
const RESERVED_AUTHORIZE_PARAMS = new Set([
  'client_id',
  'redirect_uri',
  'response_type',
  'scope',
  'state',
  'nonce',
  'code_challenge',
  'code_challenge_method',
])

/**
 * Reads a query-string-encoded variable into a parameter map, e.g.
 * `resource=https://api.example&prompt=consent`. Unset yields an empty map.
 */
export function getQueryParamsEnv(name: string): Record<string, string> {
  const raw = getEnv(name)
  if (!raw || raw.trim() === '') {
    return {}
  }

  const params: Record<string, string> = {}
  for (const [key, value] of new URLSearchParams(raw)) {
    if (RESERVED_AUTHORIZE_PARAMS.has(key)) {
      throw new Error(`${name}: "${key}" is set by the OIDC client and must not be overridden`)
    }
    params[key] = value
  }

  return params
}

export function getBooleanEnv(name: string, fallback = false): boolean {
  const value = getEnv(name)
  if (!value) {
    return fallback
  }

  return value.toLowerCase() === 'true'
}

export function getIdpRoleGroupConfig(): IdpRoleGroupConfig {
  return {
    traderGroupName: getEnv('VITE_IDP_TRADER_GROUP_NAME', DEFAULT_TRADER_GROUP_NAME),
    chaGroupName: getEnv('VITE_IDP_CHA_GROUP_NAME', DEFAULT_CHA_GROUP_NAME),
  }
}
