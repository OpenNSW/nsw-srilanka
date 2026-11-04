import { createContext } from 'react'
import { isAppearanceSetting, type Appearance, type AppearanceSetting } from '@portals/theme'

// Where the user's choice is kept. The script in index.html reads the same
// key, with the same rule, to set the appearance before the app loads.
export const APPEARANCE_STORAGE_KEY = 'appearance'

// The user's choice, or the OS's appearance when they haven't made one.
export function storedAppearanceSetting(): AppearanceSetting {
  try {
    const value = localStorage.getItem(APPEARANCE_STORAGE_KEY)
    return isAppearanceSetting(value) ? value : 'system'
  } catch {
    return 'system'
  }
}

export function storeAppearanceSetting(setting: AppearanceSetting): void {
  try {
    localStorage.setItem(APPEARANCE_STORAGE_KEY, setting)
  } catch {
    // Storage blocked (a private window, a strict policy): the choice lasts
    // until the page is reloaded.
  }
}

export interface AppearanceContextType {
  // What the user chose.
  setting: AppearanceSetting
  // What the page is in: the choice, or the OS's appearance for 'system'.
  appearance: Appearance
  setSetting: (setting: AppearanceSetting) => void
}

export const AppearanceContext = createContext<AppearanceContextType | undefined>(undefined)
