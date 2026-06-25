import { validateConfig, getDisplayName } from './configs/types'
import type { UIConfig } from './configs/types'

export let appConfig: UIConfig = {} as UIConfig
export let displayName = ''

export async function initAppConfig(): Promise<void> {
  try {
    const res = await fetch('/configs/branding.json')
    if (!res.ok) {
      throw new Error(`Failed to load branding configuration: ${res.statusText}`)
    }

    const configData = (await res.json()) as Record<string, unknown>
    appConfig = validateConfig(configData, 'branding.json')
    displayName = getDisplayName(appConfig)
  } catch (error) {
    console.error('Failed to load branding configuration:', error)
    // Absolute fallback to prevent breaking the application completely
    appConfig = {
      branding: {
        systemName: 'NSW',
        appName: 'Trader Portal',
      },
    }
    displayName = 'Trader Portal'
  }
}
