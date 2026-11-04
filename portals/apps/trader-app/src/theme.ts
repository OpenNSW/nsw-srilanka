import type { ThemeConfig } from '@portals/theme'

// The portal's colors when branding.json sets no theme: the Maritime navy it
// has shipped with, on Radix's blue and slate scales.
export const DEFAULT_THEME: ThemeConfig = {
  accent: 'blue',
  gray: 'slate',
  colors: {
    'accent-1': '#fdfdfe',
    'accent-2': '#f8f9fb',
    'accent-3': '#eff1f7',
    'accent-4': '#e6e9f2',
    'accent-5': '#dbdfec',
    'accent-6': '#ced4e5',
    'accent-7': '#bcc4dc',
    'accent-8': '#9aa6ca',
    'accent-9': '#1e3a8a',
    'accent-10': '#1a3175',
    // Text on the accent's own soft tints: the hover step, for 4.5:1.
    'accent-11': '#1a3175',
    'accent-12': '#0e1a3e',
    'accent-a1': '#1e3a8a03',
    'accent-a2': '#1e3a8a08',
    'accent-a3': '#1e3a8a12',
    'accent-a4': '#1e3a8a1c',
    'accent-a5': '#1e3a8a29',
    'accent-a6': '#1e3a8a38',
    'accent-a7': '#1e3a8a4c',
    'accent-a8': '#1e3a8a73',
    'accent-a9': '#1e3a8a',
    'accent-a10': '#1a3175',
    'accent-a11': '#1a3175',
    'accent-a12': '#0e1a3e',
    'accent-contrast': '#ffffff',
    'accent-surface': '#f6f7fa',
    'accent-indicator': '#1e3a8a',
    'accent-track': '#1e3a8a',
  },
}

// Status text (soft badges, colored Text) in light. Radix's step 11 for
// these hues reads at 3.6-4.3:1 on a soft badge over the page background;
// each is darkened just enough for 4.5:1, keeping its hue. Under every
// theme, which can set them itself. They change the raw scale, so a theme
// whose accent is one of these hues gets them as its accent-11 and
// accent-a11 too, for the same contrast on the accent's soft tints. They
// apply to light only; dark uses Radix's own.
const STATUS_TEXT_COLORS: Record<string, string> = {
  'blue-11': '#0b66b6',
  'blue-a11': '#0b66b6',
  'orange-11': '#b04300',
  'orange-a11': '#b04300',
  'green-11': '#1b734c',
  'green-a11': '#1b734c',
  'red-11': '#c1262b',
  'red-a11': '#c1262b',
  'amber-11': '#995a00',
  'amber-a11': '#995a00',
}

// The theme to paint: the deployment's, or the default, with the status
// text fixes under it.
export function appTheme(configured: ThemeConfig | undefined): ThemeConfig {
  const theme = configured ?? DEFAULT_THEME
  return { ...theme, colors: { ...STATUS_TEXT_COLORS, ...theme.colors } }
}
