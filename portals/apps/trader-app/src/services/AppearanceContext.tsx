import { useCallback, useLayoutEffect, useMemo, useState, useSyncExternalStore, type ReactNode } from 'react'
import { applyAppearance, systemAppearance, watchSystemAppearance, type AppearanceSetting } from '@portals/theme'
import { AppearanceContext, storeAppearanceSetting, storedAppearanceSetting } from './appearanceContextCore'

// Light, dark or the OS's appearance, as the user chose; the OS's until they
// choose. Puts the document in that appearance, and follows the OS live
// while the choice is 'system'.
export function AppearanceProvider({ children }: { children: ReactNode }) {
  const [setting, setSettingState] = useState(storedAppearanceSetting)
  // The OS's appearance, kept current. React reads it again once subscribed,
  // so a switch between the first render and the subscription isn't missed.
  const system = useSyncExternalStore(watchSystemAppearance, () => systemAppearance())
  const appearance = setting === 'system' ? system : setting

  // A layout effect runs before the browser paints, so a change never shows
  // a frame in the old appearance.
  useLayoutEffect(() => applyAppearance(appearance), [appearance])

  const setSetting = useCallback((next: AppearanceSetting) => {
    setSettingState(next)
    storeAppearanceSetting(next)
  }, [])

  const value = useMemo(() => ({ setting, appearance, setSetting }), [setting, appearance, setSetting])

  return <AppearanceContext.Provider value={value}>{children}</AppearanceContext.Provider>
}
