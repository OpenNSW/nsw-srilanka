import { describe, expect, it } from 'vitest'
import { validateConfig } from './types'

const branding = { systemName: 'NSW', appName: 'Trader Portal' }

describe('validateConfig theme', () => {
  it('accepts a theme of Radix scales and steps', () => {
    const config = validateConfig(
      {
        branding: {
          ...branding,
          theme: {
            accent: 'ruby',
            gray: 'sand',
            colors: { 'accent-9': '#7a2430' },
            dark: { colors: { 'accent-a3': 'x' } },
          },
        },
      },
      'branding.json',
    )

    expect(config.branding.theme?.colors).toEqual({ 'accent-9': '#7a2430' })
  })

  it('rejects a color name that is not a Radix step', () => {
    expect(() =>
      validateConfig({ branding: { ...branding, theme: { colors: { 'acent-9': '#7a2430' } } } }, 'branding.json'),
    ).toThrow(/branding\.theme\.colors\.acent-9: must be a Radix step/)
  })

  it('rejects a scale Radix does not ship', () => {
    expect(() => validateConfig({ branding: { ...branding, theme: { accent: 'navy' } } }, 'branding.json')).toThrow(
      /branding\.theme\.accent/,
    )
  })
})
