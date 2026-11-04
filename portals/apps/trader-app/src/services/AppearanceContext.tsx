import { useCallback, useEffect, useLayoutEffect, useMemo, useState, type ReactNode } from 'react'
import { applyAppearance, systemAppearance, watchSystemAppearance, type AppearanceSetting } from '@portals/theme'
import { AppearanceContext, storeAppearanceSetting, storedAppearanceSetting } from './appearanceContextCore'

// Light, dark or the OS's appearance, as the user chose; the OS's until they
// choose. Puts the document in that appearance, and follows the OS live
// while the choice is 'system'.
export function AppearanceProvider({ children }: { children: ReactNode }) {
  const [setting, setSettingState] = useState(storedAppearanceSetting)
  const [system, setSystem] = useState(() => systemAppearance())
  const appearance = setting === 'system' ? system : setting

  useEffect(() => watchSystemAppearance(setSystem), [])

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
