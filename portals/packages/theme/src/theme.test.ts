import { afterEach, describe, expect, it } from 'vitest'
import { DEFAULT_THEME, applyTheme, themeVariables } from './theme'

function rules() {
  const style = document.getElementById('portals-theme') as HTMLStyleElement
  return Array.from(style.sheet!.cssRules) as CSSStyleRule[]
}

afterEach(() => {
  document.getElementById('portals-theme')?.remove()
})

describe('themeVariables', () => {
  it('points every step of a named scale at that Radix scale', () => {
    const vars = themeVariables({ accent: 'ruby', gray: 'sand' }, 'light')

    expect(vars['accent-1']).toBe('var(--ruby-1)')
    expect(vars['accent-a12']).toBe('var(--ruby-a12)')
    expect(vars['accent-contrast']).toBe('var(--ruby-contrast)')
    expect(vars['gray-9']).toBe('var(--sand-9)')
    expect(vars['gray-track']).toBe('var(--sand-track)')
  })

  it('lets colors replace single steps of a named scale', () => {
    const vars = themeVariables({ accent: 'blue', colors: { 'accent-9': '#1e3a8a' } }, 'light')

    expect(vars['accent-9']).toBe('#1e3a8a')
    expect(vars['accent-8']).toBe('var(--blue-8)')
  })

  it('sets a step with no named scale', () => {
    expect(themeVariables({ colors: { 'accent-9': '#123456' } }, 'light')).toMatchObject({ 'accent-9': '#123456' })
  })

  it('darkens the status text steps in light, unless the theme sets them', () => {
    expect(themeVariables({}, 'light')['red-11']).toBe('#c1262b')
    expect(themeVariables({ colors: { 'red-11': '#aa0000' } }, 'light')['red-11']).toBe('#aa0000')
  })

  it('keeps the light scales but not the light colors in dark', () => {
    const vars = themeVariables({ accent: 'jade', gray: 'sage', colors: { 'accent-9': '#0f6a52' } }, 'dark')

    expect(vars['accent-9']).toBe('var(--jade-9)')
    expect(vars['gray-1']).toBe('var(--sage-1)')
    expect(vars).not.toHaveProperty('red-11')
  })

  it('uses the dark palette in dark', () => {
    const vars = themeVariables(
      { accent: 'blue', dark: { accent: 'sky', gray: 'mauve', colors: { 'accent-11': '#a9bdf5' } } },
      'dark',
    )

    expect(vars['accent-1']).toBe('var(--sky-1)')
    expect(vars['gray-1']).toBe('var(--mauve-1)')
    expect(vars['accent-11']).toBe('#a9bdf5')
  })
})

describe('applyTheme', () => {
  it('paints the default theme when given none', () => {
    applyTheme()

    const [light, dark] = rules()
    expect(light.selectorText).toContain('.radix-themes:not(.dark)')
    expect(light.style.getPropertyValue('--accent-9')).toBe(DEFAULT_THEME.colors!['accent-9'])
    expect(light.style.getPropertyValue('--gray-6')).toBe('var(--slate-6)')
    expect(dark.selectorText).toContain('.radix-themes.dark')
    expect(dark.style.getPropertyValue('--accent-9')).toBe('var(--blue-9)')
  })

  it('replaces the theme it painted before', () => {
    applyTheme({ accent: 'ruby' })
    applyTheme({ accent: 'jade' })

    expect(document.querySelectorAll('#portals-theme')).toHaveLength(1)
    expect(rules()).toHaveLength(2)
    expect(rules()[0].style.getPropertyValue('--accent-1')).toBe('var(--jade-1)')
  })

  it('keeps a value from breaking out of its rule', () => {
    applyTheme({ colors: { 'accent-9': 'red; } body { display: none' } })

    expect(rules()).toHaveLength(2)
    expect(rules()[0].style.getPropertyValue('display')).toBe('')
  })
})
