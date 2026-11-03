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

const VARIABLE_SCALES = new Set<string>(['accent', ...ACCENT_SCALES, ...GRAY_SCALES])

// Whether a theme's `colors` can set this variable: a step of the accent or
// gray role, or of any Radix scale ('accent-9', 'gray-a3', 'red-11',
// 'blue-contrast'). Only the name is checked, never the value.
export function isThemeVariable(name: string): boolean {
  const dash = name.lastIndexOf('-')
  return VARIABLE_SCALES.has(name.slice(0, dash)) && STEPS.includes(name.slice(dash + 1))
}

// Each appearance's variables are set on <html>, where the Tailwind tokens
// resolve, and on every Theme root (.radix-themes: the app's and each dialog
// portal's), which Radix's own [data-accent-color] and [data-gray-color]
// rules would otherwise repaint. Both selectors outrank Radix's without
// !important. A Theme root takes the dark variables when it is dark itself
// or sits under a dark <html> without being light; the light ones never
// reach it then, so a value set for light can't leak into dark.
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
      ? config
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
    // Radix's own --gray-N already are the gray scale; pointing them at
    // themselves would be a cycle, which the browser treats as invalid. For
    // a Theme root to keep them, it must be given grayColor="gray", which
    // Radix doesn't remap.
    if (!scale || (role === 'gray' && scale === 'gray')) continue
    for (const step of STEPS) vars[`${role}-${step}`] = `var(--${scale}-${step})`
  }
  return { ...vars, ...palette.colors }
}

// Paints the theme into the document, replacing whatever theme was there.
// An empty theme leaves Radix's own scales.
// Values go in through the CSSOM rather than as stylesheet text, so a value
// can't break out of its rule, and one the browser can't parse is dropped
// on its own.
export function applyTheme(config: ThemeConfig, doc: Document = document): void {
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
