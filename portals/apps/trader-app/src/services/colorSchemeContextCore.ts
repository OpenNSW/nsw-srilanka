import { createContext } from 'react'
import type { ThemeProps } from '@radix-ui/themes'
import { getBooleanEnv } from '@/runtimeConfig'

// The named Radix palette nearest each scheme's --color-primary. Radix's
// accent scale itself is derived from --color-primary in index.css (the
// "Radix accent" block), so buttons and JSONForms controls are painted in
// the exact brand color; this name is only what <Theme> is given.
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

// The scheme switcher is a demo control: a deployment shows it in the footer
// only with SHOW_COLOR_SCHEME_SWITCHER set to 'true'. Without it, every
// visitor gets the default scheme, whatever an earlier visit chose.
export function showColorSchemeSwitcher(): boolean {
  return getBooleanEnv('SHOW_COLOR_SCHEME_SWITCHER')
}
