import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { Theme } from '@radix-ui/themes'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { CopyableCode } from './CopyableCode'

function stubClipboard(writeText: (text: string) => Promise<void>) {
  Object.defineProperty(navigator, 'clipboard', { value: { writeText }, configurable: true })
}

describe('CopyableCode', () => {
  afterEach(() => {
    Object.defineProperty(navigator, 'clipboard', { value: undefined, configurable: true })
    vi.restoreAllMocks()
  })

  it('shows the value and a button that names it', () => {
    stubClipboard(vi.fn().mockResolvedValue(undefined))
    render(
      <Theme>
        <CopyableCode>034/25236</CopyableCode>
      </Theme>,
    )

    expect(screen.getByText('034/25236')).toBeTruthy()
    expect(screen.getByRole('button', { name: 'Copy 034/25236' })).toBeTruthy()
  })

  it('copies the value and confirms it', async () => {
    const writeText = vi.fn().mockResolvedValue(undefined)
    stubClipboard(writeText)
    render(
      <Theme>
        <CopyableCode>TNSWU21F5FM6</CopyableCode>
      </Theme>,
    )

    fireEvent.click(screen.getByRole('button', { name: 'Copy TNSWU21F5FM6' }))

    expect(writeText).toHaveBeenCalledWith('TNSWU21F5FM6')
    await waitFor(() => expect(screen.getByRole('button').querySelector('svg')).toBeTruthy())
  })

  it('shows only the value when the clipboard is unavailable', () => {
    Object.defineProperty(navigator, 'clipboard', { value: undefined, configurable: true })
    render(
      <Theme>
        <CopyableCode>TNSWU21F5FM6</CopyableCode>
      </Theme>,
    )

    expect(screen.getByText('TNSWU21F5FM6')).toBeTruthy()
    expect(screen.queryByRole('button')).toBeNull()
  })

  it('logs a failed copy without throwing', async () => {
    const error = vi.spyOn(console, 'error').mockImplementation(() => {})
    stubClipboard(vi.fn().mockRejectedValue(new Error('denied')))
    render(
      <Theme>
        <CopyableCode>TNSWU21F5FM6</CopyableCode>
      </Theme>,
    )

    fireEvent.click(screen.getByRole('button', { name: 'Copy TNSWU21F5FM6' }))

    await waitFor(() => expect(error).toHaveBeenCalled())
  })
})
