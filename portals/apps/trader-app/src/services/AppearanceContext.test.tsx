import { afterEach, describe, expect, it } from 'vitest'
import { act, render, screen } from '@testing-library/react'
import { AppearanceProvider } from './AppearanceContext'
import { APPEARANCE_STORAGE_KEY } from './appearanceContextCore'
import { useAppearance } from './useAppearance'
import indexHtml from '../../index.html?raw'

function Probe() {
  const { setting, appearance, setSetting } = useAppearance()
  return (
    <>
      <output>{`${setting}:${appearance}`}</output>
      <button onClick={() => setSetting('dark')}>dark</button>
      <button onClick={() => setSetting('system')}>system</button>
    </>
  )
}

function renderProbe() {
  render(
    <AppearanceProvider>
      <Probe />
    </AppearanceProvider>,
  )
  return () => screen.getByRole('status').textContent
}

afterEach(() => {
  localStorage.clear()
  document.documentElement.className = ''
  document.documentElement.style.colorScheme = ''
})

describe('AppearanceProvider', () => {
  // jsdom has no matchMedia, so the OS reads as light.
  it("follows the OS until the user chooses, and the OS's is light here", () => {
    const shown = renderProbe()
    expect(shown()).toBe('system:light')
    expect(document.documentElement.classList.contains('dark')).toBe(false)
    expect(document.documentElement.style.colorScheme).toBe('light')
  })

  it('applies and remembers a choice', () => {
    const shown = renderProbe()
    act(() => screen.getByRole('button', { name: 'dark' }).click())

    expect(shown()).toBe('dark:dark')
    expect(document.documentElement.classList.contains('dark')).toBe(true)
    expect(localStorage.getItem(APPEARANCE_STORAGE_KEY)).toBe('dark')

    act(() => screen.getByRole('button', { name: 'system' }).click())
    expect(shown()).toBe('system:light')
    expect(document.documentElement.classList.contains('dark')).toBe(false)
  })

  it('starts from the remembered choice', () => {
    localStorage.setItem(APPEARANCE_STORAGE_KEY, 'dark')
    expect(renderProbe()()).toBe('dark:dark')
    expect(document.documentElement.classList.contains('dark')).toBe(true)
  })

  it('ignores a stored value that is not a setting', () => {
    localStorage.setItem(APPEARANCE_STORAGE_KEY, 'midnight')
    expect(renderProbe()()).toBe('system:light')
  })
})

describe('index.html', () => {
  // It sets the appearance before the app loads, so it must read the
  // provider's storage key.
  it("reads the provider's storage key before first paint", () => {
    expect(indexHtml).toContain(`localStorage.getItem('${APPEARANCE_STORAGE_KEY}')`)
  })
})
