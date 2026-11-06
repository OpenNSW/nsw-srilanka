import { render, screen } from '@testing-library/react'
import { Theme } from '@radix-ui/themes'
import { afterEach, describe, expect, it } from 'vitest'
import { MarkdownRenderer } from './MarkdownRenderer'

describe('MarkdownRenderer', () => {
  afterEach(() => {
    Object.defineProperty(navigator, 'clipboard', { value: undefined, configurable: true })
  })

  it('makes inline code copyable', () => {
    Object.defineProperty(navigator, 'clipboard', {
      value: { writeText: () => Promise.resolve() },
      configurable: true,
    })
    render(
      <Theme>
        <MarkdownRenderer payload={{ content: '- **FCAU Reference Number**: `034/25236`' }} />
      </Theme>,
    )

    expect(screen.getByText('034/25236')).toBeTruthy()
    expect(screen.getByRole('button', { name: 'Copy 034/25236' })).toBeTruthy()
  })
})
