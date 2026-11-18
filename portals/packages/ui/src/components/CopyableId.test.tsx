import { fireEvent, render, screen } from '@testing-library/react'
import { Theme } from '@radix-ui/themes'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { CopyableId } from './CopyableId'

describe('CopyableId', () => {
  afterEach(() => {
    Object.defineProperty(navigator, 'clipboard', { value: undefined, configurable: true })
  })

  it('shows the label and ID, and copies the ID', () => {
    const writeText = vi.fn().mockResolvedValue(undefined)
    Object.defineProperty(navigator, 'clipboard', { value: { writeText }, configurable: true })
    render(
      <Theme>
        <CopyableId label="Task ID" id="21f9ad17-ff66-5296-b125-385b9ca93f77" />
      </Theme>,
    )

    expect(screen.getByText('Task ID')).toBeTruthy()
    expect(screen.getByText('21f9ad17-ff66-5296-b125-385b9ca93f77')).toBeTruthy()
    fireEvent.click(screen.getByRole('button', { name: 'Copy Task ID' }))
    expect(writeText).toHaveBeenCalledWith('21f9ad17-ff66-5296-b125-385b9ca93f77')
  })

  it('shows the ID with no button when the clipboard is unavailable', () => {
    render(
      <Theme>
        <CopyableId label="Task ID" id="21f9ad17-ff66-5296-b125-385b9ca93f77" />
      </Theme>,
    )

    expect(screen.getByText('21f9ad17-ff66-5296-b125-385b9ca93f77')).toBeTruthy()
    expect(screen.queryByRole('button')).toBeNull()
  })

  it('lets a long ID break across lines', () => {
    render(
      <Theme>
        <CopyableId label="Task ID" id="21f9ad17-ff66-5296-b125-385b9ca93f77" />
      </Theme>,
    )

    expect(screen.getByText('21f9ad17-ff66-5296-b125-385b9ca93f77').className).toContain('break-all')
    expect(screen.getByText('Task ID').className).toContain('shrink-0')
  })
})
