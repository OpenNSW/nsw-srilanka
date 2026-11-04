import { describe, expect, it } from 'vitest'
import { DEFAULT_THEME, appTheme } from './theme'

describe('appTheme', () => {
  it('uses the default theme when branding.json sets none', () => {
    const theme = appTheme(undefined)

    expect(theme.accent).toBe('blue')
    expect(theme.colors?.['accent-9']).toBe(DEFAULT_THEME.colors?.['accent-9'])
  })

  it("uses the deployment's theme in place of the default", () => {
    const theme = appTheme({ accent: 'ruby' })

    expect(theme.accent).toBe('ruby')
    expect(theme.colors).not.toHaveProperty('accent-9')
  })

  it('puts the status text fixes under every theme, which can replace them', () => {
    expect(appTheme(undefined).colors?.['red-11']).toBe('#c1262b')
    expect(appTheme({ accent: 'ruby' }).colors?.['red-11']).toBe('#c1262b')
    expect(appTheme({ colors: { 'red-11': '#aa0000' } }).colors?.['red-11']).toBe('#aa0000')
  })

  // WCAG contrast of white text on a #rrggbb fill.
  function contrastWithWhite(hex: string): number {
    const [r, g, b] = [1, 3, 5].map((i) => {
      const c = parseInt(hex.slice(i, i + 2), 16) / 255
      return c <= 0.04045 ? c / 12.92 : ((c + 0.055) / 1.055) ** 2.4
    })
    const luminance = 0.2126 * r + 0.7152 * g + 0.0722 * b
    return 1.05 / (luminance + 0.05)
  }

  it.each(['accent-9', 'accent-10'] as const)('carries white text at 4.5:1 on its %s in both appearances', (step) => {
    for (const colors of [DEFAULT_THEME.colors, DEFAULT_THEME.dark?.colors]) {
      expect(contrastWithWhite(colors?.[step] ?? '')).toBeGreaterThanOrEqual(4.5)
    }
  })

  it('keeps the dark palette as configured', () => {
    expect(appTheme({ dark: { accent: 'sky' } }).dark).toEqual({ accent: 'sky' })
  })
})
