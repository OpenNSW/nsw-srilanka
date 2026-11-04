import { afterEach, describe, expect, it, vi } from 'vitest'
import {
  applyAppearance,
  isAppearanceSetting,
  resolveAppearance,
  systemAppearance,
  watchSystemAppearance,
} from './appearance'

// A window whose OS appearance can be switched, as matchMedia reports it.
function fakeWindow(dark: boolean) {
  const listeners = new Set<(event: MediaQueryListEvent) => void>()
  const query = {
    get matches() {
      return dark
    },
    addEventListener: (_: string, listener: (event: MediaQueryListEvent) => void) => listeners.add(listener),
    removeEventListener: (_: string, listener: (event: MediaQueryListEvent) => void) => listeners.delete(listener),
  }
  const win = { matchMedia: () => query } as unknown as Window
  return {
    win,
    listeners,
    switchTo(next: boolean) {
      dark = next
      listeners.forEach((listener) => listener({ matches: next } as MediaQueryListEvent))
    },
  }
}

afterEach(() => {
  document.documentElement.className = ''
  document.documentElement.style.colorScheme = ''
})

describe('resolveAppearance', () => {
  it('keeps an explicit appearance whatever the OS uses', () => {
    expect(resolveAppearance('light', fakeWindow(true).win)).toBe('light')
    expect(resolveAppearance('dark', fakeWindow(false).win)).toBe('dark')
  })

  it('follows the OS for system', () => {
    expect(resolveAppearance('system', fakeWindow(true).win)).toBe('dark')
    expect(resolveAppearance('system', fakeWindow(false).win)).toBe('light')
  })

  it('treats a window without matchMedia as light', () => {
    expect(systemAppearance({} as Window)).toBe('light')
  })
})

describe('isAppearanceSetting', () => {
  it.each(['light', 'dark', 'system'])('accepts %s', (value) => {
    expect(isAppearanceSetting(value)).toBe(true)
  })

  it.each(['Dark', 'auto', '', null, undefined, 1])('rejects %s', (value) => {
    expect(isAppearanceSetting(value)).toBe(false)
  })
})

describe('applyAppearance', () => {
  it('marks <html> dark, and unmarks it for light', () => {
    applyAppearance('dark')
    expect(document.documentElement.classList.contains('dark')).toBe(true)
    expect(document.documentElement.style.colorScheme).toBe('dark')

    applyAppearance('light')
    expect(document.documentElement.classList.contains('dark')).toBe(false)
    expect(document.documentElement.style.colorScheme).toBe('light')
  })

  it('leaves the other classes on <html> alone', () => {
    document.documentElement.className = 'keep'
    applyAppearance('dark')
    expect(document.documentElement.className).toBe('keep dark')
  })
})

describe('watchSystemAppearance', () => {
  it('reports each OS switch until stopped', () => {
    const os = fakeWindow(false)
    const onChange = vi.fn()
    const stop = watchSystemAppearance(onChange, os.win)

    os.switchTo(true)
    os.switchTo(false)
    expect(onChange.mock.calls).toEqual([['dark'], ['light']])

    stop()
    expect(os.listeners.size).toBe(0)
  })

  it('does nothing without matchMedia', () => {
    const stop = watchSystemAppearance(vi.fn(), {} as Window)
    expect(stop).not.toThrow()
  })
})
