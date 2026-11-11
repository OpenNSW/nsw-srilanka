import { fireEvent, render, screen } from '@testing-library/react'
import { Theme } from '@radix-ui/themes'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { MarkdownRenderer } from './MarkdownRenderer'

function renderMarkdown(content: string) {
  render(
    <Theme>
      <MarkdownRenderer payload={{ content }} />
    </Theme>,
  )
}

describe('MarkdownRenderer', () => {
  const writeText = vi.fn().mockResolvedValue(undefined)

  beforeEach(() => {
    writeText.mockClear()
    Object.defineProperty(navigator, 'clipboard', { value: { writeText }, configurable: true })
  })

  afterEach(() => {
    Object.defineProperty(navigator, 'clipboard', { value: undefined, configurable: true })
  })

  it('shows a copy: link as code with a button that copies it', () => {
    renderMarkdown('- **FCAU Reference Number**: [`034/25236`](copy:)')

    expect(screen.getByText('034/25236').tagName).toBe('CODE')
    expect(screen.queryByRole('link')).toBeNull()
    fireEvent.click(screen.getByRole('button', { name: 'Copy 034/25236' }))
    expect(writeText).toHaveBeenCalledWith('034/25236')
  })

  it('copies a copy: link’s value exactly as written in its code', () => {
    renderMarkdown('[`SO_12_A*`](copy:)')

    fireEvent.click(screen.getByRole('button', { name: 'Copy SO_12_A*' }))
    expect(writeText).toHaveBeenCalledWith('SO_12_A*')
  })

  it('copies the text of a copy: link written without code', () => {
    renderMarkdown('[**TNSWU21F5FM6**](copy:)')

    expect(screen.getByText('TNSWU21F5FM6').tagName).toBe('CODE')
    fireEvent.click(screen.getByRole('button', { name: 'Copy TNSWU21F5FM6' }))
    expect(writeText).toHaveBeenCalledWith('TNSWU21F5FM6')
  })

  it('shows code outside a copy: link with no button', () => {
    renderMarkdown('* **Amount Paid**: `LKR 5000.00`')

    expect(screen.getByText('LKR 5000.00').tagName).toBe('CODE')
    expect(screen.queryByRole('button')).toBeNull()
  })

  it('shows nothing to copy for a copy: link with no text', () => {
    renderMarkdown('Reference: [](copy:)')

    expect(screen.queryByRole('button')).toBeNull()
    expect(screen.queryByRole('link')).toBeNull()
  })

  it('opens other links in a new tab', () => {
    renderMarkdown('[GovPay](https://govpay.lk)')

    const link = screen.getByRole('link', { name: 'GovPay' })
    expect(link.getAttribute('href')).toBe('https://govpay.lk')
    expect(link.getAttribute('target')).toBe('_blank')
  })
})
