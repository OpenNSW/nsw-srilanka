import { createContext, useContext, useMemo, type ReactNode } from 'react'
import { userManager } from '../oidcUserManager'
import { createApiClient, type ApiClient } from './api'

const ApiContext = createContext<ApiClient | null>(null)

export function ApiProvider({ children }: { children: ReactNode }) {
  const client = useMemo(
    () =>
      createApiClient(async () => {
        const user = await userManager.getUser()
        return user?.access_token
      }),
    [],
  )

  return <ApiContext.Provider value={client}>{children}</ApiContext.Provider>
}

export function useApi(): ApiClient {
  const apiClient = useContext(ApiContext)
  if (!apiClient) {
    throw new Error('useApi must be used within ApiProvider')
  }
  return apiClient
}
