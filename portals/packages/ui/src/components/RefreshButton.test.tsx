import { fireEvent, render, screen } from '@testing-library/react'
import { Theme } from '@radix-ui/themes'
import { describe, expect, it, vi } from 'vitest'
import { RefreshButton } from './RefreshButton'

describe('RefreshButton', () => {
  it('reloads on click, named by its label', () => {
    const onClick = vi.fn()
    render(
      <Theme>
        <RefreshButton label="Refresh" refreshing={false} onClick={onClick} />
      </Theme>,
    )

    fireEvent.click(screen.getByRole('button', { name: 'Refresh' }))
    expect(onClick).toHaveBeenCalledOnce()
  })

  it('spins and is disabled while refreshing', () => {
    render(
      <Theme>
        <RefreshButton label="Refresh" refreshing onClick={() => {}} />
      </Theme>,
    )

    const button = screen.getByRole<HTMLButtonElement>('button', { name: 'Refresh' })
    expect(button.disabled).toBe(true)
    expect(button.querySelector('svg')?.getAttribute('class')).toContain('animate-spin')
  })

  it('shows its label as text with showLabel', () => {
    const onClick = vi.fn()
    render(
      <Theme>
        <RefreshButton label="Refresh" refreshing={false} onClick={onClick} showLabel />
      </Theme>,
    )

    const button = screen.getByRole('button', { name: 'Refresh' })
    expect(button.textContent).toBe('Refresh')
    fireEvent.click(button)
    expect(onClick).toHaveBeenCalledOnce()
  })
})
