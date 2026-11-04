import { afterEach, describe, expect, it } from 'vitest'
import { fireEvent, render, screen } from '@testing-library/react'
import '@/i18n'
import { AppearanceProvider } from '@/services/AppearanceContext'
import { AppearanceSwitcher } from './AppearanceSwitcher'

afterEach(() => {
  localStorage.clear()
  document.documentElement.className = ''
})

describe('AppearanceSwitcher', () => {
  it('starts at system, then each press moves to light, dark and back to system', () => {
    render(
      <AppearanceProvider>
        <AppearanceSwitcher />
      </AppearanceProvider>,
    )
    const button = () => screen.getByRole('button')

    expect(button().getAttribute('aria-label')).toBe('Appearance: System')

    fireEvent.click(button())
    expect(button().getAttribute('aria-label')).toBe('Appearance: Light')
    expect(document.documentElement.classList.contains('dark')).toBe(false)

    fireEvent.click(button())
    expect(button().getAttribute('aria-label')).toBe('Appearance: Dark')
    expect(document.documentElement.classList.contains('dark')).toBe(true)

    fireEvent.click(button())
    expect(button().getAttribute('aria-label')).toBe('Appearance: System')
  })
})
