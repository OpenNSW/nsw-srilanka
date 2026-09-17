import { useEffect, useState, type ReactNode } from 'react'
import {
  ColorSchemeContext,
  COLOR_SCHEMES,
  DEFAULT_COLOR_SCHEME_ID,
} from './colorSchemeContextCore'

const STORAGE_KEY = 'color-scheme'

function resolveScheme(id: string | null) {
  return COLOR_SCHEMES.find((s) => s.id === id) ?? COLOR_SCHEMES.find((s) => s.id === DEFAULT_COLOR_SCHEME_ID)!
}

export function ColorSchemeProvider({ children }: { children: ReactNode }) {
  const [scheme, setScheme] = useState(() => resolveScheme(localStorage.getItem(STORAGE_KEY)))

  // The scheme's CSS variable overrides live under `[data-theme="<id>"]` in
  // index.css; this is the one place that attribute gets set, so every
  // Tailwind utility reading --color-primary etc. picks it up automatically.
  useEffect(() => {
    document.documentElement.dataset.theme = scheme.id
  }, [scheme.id])

  const setSchemeId = (id: string) => {
    const next = resolveScheme(id)
    setScheme(next)
    localStorage.setItem(STORAGE_KEY, next.id)
  }

  return <ColorSchemeContext.Provider value={{ scheme, setSchemeId }}>{children}</ColorSchemeContext.Provider>
}
