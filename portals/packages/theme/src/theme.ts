// Paints a deployment's colours into the CSS variables Radix Themes and
// @portals/ui read (--accent-N, --gray-N, and any other Radix step). It
// generates and checks nothing: whoever configures the theme supplies the
// values, and the browser ignores one it can't parse.

// The Radix scales a theme can name. Radix ships light and dark steps for
// each, so naming one themes both appearances.
// prettier-ignore
export const ACCENT_SCALES = ['gray', 'gold', 'bronze', 'brown', 'yellow', 'amber', 'orange', 'tomato', 'red',
  'ruby', 'crimson', 'pink', 'plum', 'purple', 'violet', 'iris', 'indigo', 'blue', 'cyan', 'teal', 'jade', 'green',
  'grass', 'lime', 'mint', 'sky'] as const
export const GRAY_SCALES = ['gray', 'mauve', 'slate', 'sage', 'olive', 'sand'] as const

export type AccentScale = (typeof ACCENT_SCALES)[number]
export type GrayScale = (typeof GRAY_SCALES)[number]

export interface Palette {
  // The Radix scale every --accent-* step points at, before `colors`.
  accent?: AccentScale
  // The Radix scale every --gray-* step points at, before `colors`.
  gray?: GrayScale
  // CSS variables by name without the leading dashes ('accent-9',
  // 'gray-11', 'red-11'), each set to the value given, as given.
  colors?: Record<string, string>
}

export interface ThemeConfig extends Palette {
  // Dark appearance. accent and gray default to the light ones; the light
  // `colors` don't carry over, since a step tuned for a light background
  // rarely suits a dark one.
  dark?: Palette
}

export type Appearance = 'light' | 'dark'

// Every step of a Radix scale, as Radix names them (--blue-1, --blue-a1,
// --blue-contrast, ...).
const STEPS = [
  ...Array.from({ length: 12 }, (_, i) => `${i + 1}`),
  ...Array.from({ length: 12 }, (_, i) => `a${i + 1}`),
  'contrast',
  'surface',
  'indicator',
  'track',
]

// What a deployment gets with no theme: the Maritime navy the portal has
// shipped with, on Radix's blue and slate scales.
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

// Applied under every light theme, before its own colors, which can replace
// them. Radix's step 11 for these hues reads at 3.6-4.3:1 on a soft badge
// over the page background; each is darkened just enough for 4.5:1, keeping
// its hue.
const LIGHT_BASE_COLORS: Record<string, string> = {
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

// Each appearance's variables are set on <html>, where the Tailwind tokens
// resolve, and on every Theme root (.radix-themes: the app's and each dialog
// portal's), which Radix's own [data-accent-color] and [data-gray-color]
// rules would otherwise repaint. Both selectors outrank Radix's without
// !important. A Theme root takes the dark variables when it is dark itself
// or sits under a dark <html> without being light; the light ones never
// reach it then, so a light-only value such as LIGHT_BASE_COLORS can't leak
// into dark.
const SELECTORS: Record<Appearance, string> = {
  light: 'html:root:not(.dark), :root .radix-themes:not(.dark)',
  dark: 'html:root.dark, :root .radix-themes.dark, :root.dark .radix-themes:not(.light)',
}

const STYLE_ID = 'portals-theme'

// The CSS variables (names without dashes) one appearance of a theme sets,
// in the order they apply: the named scales, then any colors given.
export function themeVariables(config: ThemeConfig, appearance: Appearance): Record<string, string> {
  const palette: Palette =
    appearance === 'light'
      ? { ...config, colors: { ...LIGHT_BASE_COLORS, ...config.colors } }
      : {
          accent: config.dark?.accent ?? config.accent,
          gray: config.dark?.gray ?? config.gray,
          colors: config.dark?.colors,
        }

  const vars: Record<string, string> = {}
  for (const [role, scale] of [
    ['accent', palette.accent],
    ['gray', palette.gray],
  ] as const) {
    if (!scale) continue
    for (const step of STEPS) vars[`${role}-${step}`] = `var(--${scale}-${step})`
  }
  return { ...vars, ...palette.colors }
}

// Paints the theme into the document, replacing whatever theme was there.
// Values go in through the CSSOM rather than as stylesheet text, so a value
// can't break out of its rule, and one the browser can't parse is dropped
// on its own.
export function applyTheme(config: ThemeConfig = DEFAULT_THEME, doc: Document = document): void {
  let style = doc.getElementById(STYLE_ID) as HTMLStyleElement | null
  if (!style) {
    style = doc.createElement('style')
    style.id = STYLE_ID
    doc.head.append(style)
  }
  const sheet = style.sheet
  if (!sheet) return
  while (sheet.cssRules.length) sheet.deleteRule(0)

  for (const appearance of ['light', 'dark'] as const) {
    const index = sheet.insertRule(`${SELECTORS[appearance]} {}`, sheet.cssRules.length)
    const rule = sheet.cssRules[index] as CSSStyleRule
    for (const [name, value] of Object.entries(themeVariables(config, appearance))) {
      rule.style.setProperty(`--${name}`, value)
    }
  }
}
