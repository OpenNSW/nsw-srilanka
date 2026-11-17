import { fireEvent, render, screen } from '@testing-library/react'
import { Theme } from '@radix-ui/themes'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { CopyableCode } from './CopyableCode'

describe('CopyableCode', () => {
  afterEach(() => {
    Object.defineProperty(navigator, 'clipboard', { value: undefined, configurable: true })
  })

  it('puts a button that copies the value inside the code pill', () => {
    const writeText = vi.fn().mockResolvedValue(undefined)
    Object.defineProperty(navigator, 'clipboard', { value: { writeText }, configurable: true })
    render(
      <Theme>
        <CopyableCode value="034/25236" />
      </Theme>,
    )

    const code = screen.getByText('034/25236')
    const button = screen.getByRole('button', { name: 'Copy 034/25236' })
    expect(code.tagName).toBe('CODE')
    expect(code.parentElement?.contains(button)).toBe(true)

    fireEvent.click(button)
    expect(writeText).toHaveBeenCalledWith('034/25236')
  })

  it('lets a long value break across lines', () => {
    render(
      <Theme>
        <CopyableCode value="21f9ad17-ff66-5296-b125-385b9ca93f77" />
      </Theme>,
    )

    expect(screen.getByText('21f9ad17-ff66-5296-b125-385b9ca93f77').className).toContain('break-all')
  })

  it('shows only the value when the clipboard is unavailable', () => {
    render(
      <Theme>
        <CopyableCode value="TNSWU21F5FM6" />
      </Theme>,
    )

    expect(screen.getByText('TNSWU21F5FM6')).toBeTruthy()
    expect(screen.queryByRole('button')).toBeNull()
  })
})
