import { useCallback, useEffect, useMemo, useState, type ReactNode } from 'react'
import { ColorSchemeContext, COLOR_SCHEMES } from './colorSchemeContextCore'

const STORAGE_KEY = 'color-scheme'

function resolveScheme(id: string | null) {
  return COLOR_SCHEMES.find((s) => s.id === id) ?? COLOR_SCHEMES[0]
}

export function ColorSchemeProvider({ children }: { children: ReactNode }) {
  const [scheme, setScheme] = useState(() => {
    const initial = resolveScheme(localStorage.getItem(STORAGE_KEY))
    // Set synchronously (not in the effect below) so the first paint already has the right
    // theme attribute — an effect only runs after that first paint, which would otherwise
    // flash the `:root` default colors for one frame before flipping to the stored scheme.
    document.documentElement.dataset.theme = initial.id
    return initial
  })

  // The scheme's CSS variable overrides live under `[data-theme="<id>"]` in
  // index.css; this is the one place that attribute gets set, so every
  // Tailwind utility reading --color-primary etc. picks it up automatically.
  useEffect(() => {
    document.documentElement.dataset.theme = scheme.id
  }, [scheme.id])

  const setSchemeId = useCallback((id: string) => {
    const next = resolveScheme(id)
    setScheme(next)
    localStorage.setItem(STORAGE_KEY, next.id)
  }, [])

  const value = useMemo(() => ({ scheme, setSchemeId }), [scheme, setSchemeId])

  return <ColorSchemeContext.Provider value={value}>{children}</ColorSchemeContext.Provider>
}
