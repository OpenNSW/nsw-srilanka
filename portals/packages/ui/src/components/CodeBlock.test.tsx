import { fireEvent, render, screen } from '@testing-library/react'
import { Theme } from '@radix-ui/themes'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { CodeBlock } from './CodeBlock'

const json = JSON.stringify({ fcau: { reference_number: '034/25236' } }, null, 2)

describe('CodeBlock', () => {
  afterEach(() => {
    Object.defineProperty(navigator, 'clipboard', { value: undefined, configurable: true })
  })

  it('shows the text and copies all of it, named by its label', () => {
    const writeText = vi.fn().mockResolvedValue(undefined)
    Object.defineProperty(navigator, 'clipboard', { value: { writeText }, configurable: true })
    render(
      <Theme>
        <CodeBlock label="Workflow variables">{json}</CodeBlock>
      </Theme>,
    )

    expect(screen.getByText(/"reference_number": "034\/25236"/).tagName).toBe('PRE')
    fireEvent.click(screen.getByRole('button', { name: 'Copy Workflow variables' }))
    expect(writeText).toHaveBeenCalledWith(json)
  })

  it('adds the caller’s classes to the block', () => {
    render(
      <Theme>
        <CodeBlock label="Last error" className="max-h-32">
          boom
        </CodeBlock>
      </Theme>,
    )

    expect(screen.getByText('boom').className).toContain('max-h-32')
  })

  it('shows the text with no button when the clipboard is unavailable', () => {
    render(
      <Theme>
        <CodeBlock label="Last error">boom</CodeBlock>
      </Theme>,
    )

    expect(screen.getByText('boom')).toBeTruthy()
    expect(screen.queryByRole('button')).toBeNull()
  })
})
