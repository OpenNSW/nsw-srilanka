import { afterEach, describe, expect, it } from 'vitest'
import { applyTheme, isThemeVariable, themeVariables } from './theme'

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

  it('leaves the gray role to Radix when the gray scale is gray', () => {
    const vars = themeVariables({ accent: 'gray', gray: 'gray', colors: { 'gray-11': '#555555' } }, 'light')

    expect(Object.keys(vars).filter((name) => name.startsWith('gray-'))).toEqual(['gray-11'])
    expect(vars['accent-9']).toBe('var(--gray-9)')
  })

  it('lets colors replace single steps of a named scale', () => {
    const vars = themeVariables({ accent: 'blue', colors: { 'accent-9': '#1e3a8a' } }, 'light')

    expect(vars['accent-9']).toBe('#1e3a8a')
    expect(vars['accent-8']).toBe('var(--blue-8)')
  })

  it('sets a step with no named scale', () => {
    expect(themeVariables({ colors: { 'accent-9': '#123456' } }, 'light')).toMatchObject({ 'accent-9': '#123456' })
  })

  it('sets nothing for an empty theme, leaving Radix its own scales', () => {
    expect(themeVariables({}, 'light')).toEqual({})
    expect(themeVariables({}, 'dark')).toEqual({})
  })

  it('keeps the light scales but not the light colors in dark', () => {
    const vars = themeVariables({ accent: 'jade', gray: 'sage', colors: { 'accent-9': '#0f6a52' } }, 'dark')

    expect(vars['accent-9']).toBe('var(--jade-9)')
    expect(vars['gray-1']).toBe('var(--sage-1)')
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
  it('paints each appearance under its own selectors', () => {
    applyTheme({ accent: 'blue', gray: 'slate', colors: { 'accent-9': '#1e3a8a' } })

    const [light, dark] = rules()
    expect(light.selectorText).toContain('.radix-themes:not(.dark)')
    expect(light.style.getPropertyValue('--accent-9')).toBe('#1e3a8a')
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

describe('isThemeVariable', () => {
  it.each(['accent-9', 'accent-a12', 'gray-contrast', 'red-11', 'slate-a3', 'blue-surface'])('accepts %s', (name) => {
    expect(isThemeVariable(name)).toBe(true)
  })

  it.each(['acent-9', 'accent-13', 'accent-a0', 'accent', '--accent-9', 'Accent-9', 'navy-9', 'accent-9 '])(
    'rejects %s',
    (name) => {
      expect(isThemeVariable(name)).toBe(false)
    },
  )
})
