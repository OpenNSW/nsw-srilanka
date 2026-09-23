import { createContext } from 'react'
import type { ThemeProps } from '@radix-ui/themes'

// The Radix accent color each scheme maps to. Radix ships a fixed named
// palette (no arbitrary hex), so this picks whichever built-in scale reads
// closest to the scheme's actual --color-primary (defined per scheme in
// index.css) — it drives native Radix components (buttons, checkboxes,
// focus rings, and every JSONForms field, since that renderer is built on
// Radix Themes), while the CSS variables drive our own Tailwind utilities
// (bg-primary, text-primary, etc.) for the same visual result everywhere.
export type AccentColor = NonNullable<ThemeProps['accentColor']>

export interface ColorScheme {
  id: string
  label: string
  description: string
  accentColor: AccentColor
  // Hex used only for the little preview swatch in the switcher — the real
  // color values live in index.css under `[data-theme="<id>"]`.
  swatch: string
}

// Index 0 is the default scheme (see ColorSchemeContext's resolveScheme).
export const COLOR_SCHEMES: ColorScheme[] = [
  {
    id: 'maritime',
    label: 'Maritime Trade',
    description: 'Navy & slate — a conventional trade/customs-portal palette.',
    accentColor: 'blue',
    swatch: '#1e3a8a',
  },
  {
    id: 'regal',
    label: 'Ceylon Regal',
    description: 'Maroon & gold, echoing the national flag.',
    accentColor: 'ruby',
    swatch: '#7a2430',
  },
  {
    id: 'emerald',
    label: 'Emerald Trade',
    description: 'Deep teal-green.',
    accentColor: 'jade',
    swatch: '#0f6a52',
  },
  {
    id: 'iris',
    label: 'Modern',
    description: 'The original violet-blue palette.',
    accentColor: 'iris',
    swatch: '#5b5bd6',
  },
]

export interface ColorSchemeContextType {
  scheme: ColorScheme
  setSchemeId: (id: string) => void
}

export const ColorSchemeContext = createContext<ColorSchemeContextType | undefined>(undefined)
