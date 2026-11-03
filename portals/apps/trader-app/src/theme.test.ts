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

  it('keeps the dark palette as configured', () => {
    expect(appTheme({ dark: { accent: 'sky' } }).dark).toEqual({ accent: 'sky' })
  })
})
