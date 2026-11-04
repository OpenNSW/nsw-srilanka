import type { Appearance } from './theme'

// What a user can choose: an appearance, or whatever their OS uses.
export const APPEARANCE_SETTINGS = ['light', 'dark', 'system'] as const
export type AppearanceSetting = (typeof APPEARANCE_SETTINGS)[number]

const DARK_QUERY = '(prefers-color-scheme: dark)'

function darkQuery(win: Window): MediaQueryList | undefined {
  // Missing where there's no layout engine (jsdom, server rendering).
  return typeof win.matchMedia === 'function' ? win.matchMedia(DARK_QUERY) : undefined
}

export function isAppearanceSetting(value: unknown): value is AppearanceSetting {
  return APPEARANCE_SETTINGS.includes(value as AppearanceSetting)
}

// The appearance the OS asks for; light when it can't say.
export function systemAppearance(win: Window = window): Appearance {
  return darkQuery(win)?.matches ? 'dark' : 'light'
}

export function resolveAppearance(setting: AppearanceSetting, win: Window = window): Appearance {
  return setting === 'system' ? systemAppearance(win) : setting
}

// Puts the document in an appearance. The `dark` class on <html> is what
// Radix Themes' dark scales, applyTheme's dark rules and Tailwind's `dark:`
// variant (in @portals/ui) key off; a Theme root left to inherit its
// appearance follows it. color-scheme turns the browser's own controls and
// scrollbars dark too.
export function applyAppearance(appearance: Appearance, doc: Document = document): void {
  const root = doc.documentElement
  root.classList.toggle('dark', appearance === 'dark')
  root.style.colorScheme = appearance
}

// Calls onChange whenever the OS switches appearance. Returns the function
// that stops watching.
export function watchSystemAppearance(onChange: (appearance: Appearance) => void, win: Window = window): () => void {
  const query = darkQuery(win)
  if (!query) return () => {}
  const listener = (event: MediaQueryListEvent) => onChange(event.matches ? 'dark' : 'light')
  query.addEventListener('change', listener)
  return () => query.removeEventListener('change', listener)
}
