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

  it('gives each copy: link a button that copies its own value', () => {
    renderMarkdown('* **CusDec Number**: [`CUSDEC-1`](copy:)\n* **CDN Number**: [`CDN-2`](copy:)')

    fireEvent.click(screen.getByRole('button', { name: 'Copy CDN-2' }))
    fireEvent.click(screen.getByRole('button', { name: 'Copy CUSDEC-1' }))
    expect(writeText.mock.calls).toEqual([['CDN-2'], ['CUSDEC-1']])
  })

  it('copies a copy: link’s value without the spaces around it', () => {
    renderMarkdown('[ TNSWU21F5FM6 ](copy:)')

    fireEvent.click(screen.getByRole('button', { name: 'Copy TNSWU21F5FM6' }))
    expect(writeText).toHaveBeenCalledWith('TNSWU21F5FM6')
  })

  it.each([
    ['no text', 'Reference: [](copy:)'],
    ['spaces', 'Reference: [   ](copy:)'],
    ['code of a space', 'Reference: [` `](copy:)'],
    ['an empty template value', 'Reference: [``](copy:)'],
  ])('shows nothing for a copy: link with %s', (_, content) => {
    const { container } = render(
      <Theme>
        <MarkdownRenderer payload={{ content }} />
      </Theme>,
    )

    expect(screen.queryByRole('button')).toBeNull()
    expect(screen.queryByRole('link')).toBeNull()
    expect(container.querySelector('code')).toBeNull()
    expect(container.textContent).not.toContain('`')
  })

  it('keeps a files: link a download link next to a copy: link', () => {
    renderMarkdown('* **Reference**: [`REF-9`](copy:) — [Download](files:svc:tok123)')

    expect(screen.getByRole('link', { name: 'Download' }).getAttribute('href')).toBe('/storage/svc:tok123')
    expect(screen.getByRole('button', { name: 'Copy REF-9' })).toBeTruthy()
  })

  it('shows a files: link with no file as its label', () => {
    renderMarkdown('[Download](files:)')

    expect(screen.getByText('Download')).toBeTruthy()
    expect(screen.queryByRole('link')).toBeNull()
  })

  it('opens other links in a new tab', () => {
    renderMarkdown('[GovPay](https://govpay.lk)')

    const link = screen.getByRole('link', { name: 'GovPay' })
    expect(link.getAttribute('href')).toBe('https://govpay.lk')
    expect(link.getAttribute('target')).toBe('_blank')
  })
})
