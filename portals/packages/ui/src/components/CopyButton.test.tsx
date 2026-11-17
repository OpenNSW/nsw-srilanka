import { act, fireEvent, render, screen } from '@testing-library/react'
import { Theme } from '@radix-ui/themes'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { defaultUiLabels, UiLabelsProvider } from '../labels'
import { CopyButton } from './CopyButton'

function stubClipboard(writeText?: (text: string) => Promise<void>) {
  Object.defineProperty(navigator, 'clipboard', {
    value: writeText ? { writeText } : undefined,
    configurable: true,
  })
}

describe('CopyButton', () => {
  afterEach(() => {
    stubClipboard()
    vi.useRealTimers()
    vi.restoreAllMocks()
  })

  it('copies its value, named by its label', () => {
    const writeText = vi.fn().mockResolvedValue(undefined)
    stubClipboard(writeText)
    render(
      <Theme>
        <CopyButton value="TNSWU21F5FM6" label="Reference Number" />
      </Theme>,
    )

    fireEvent.click(screen.getByRole('button', { name: 'Copy Reference Number' }))

    expect(writeText).toHaveBeenCalledWith('TNSWU21F5FM6')
  })

  it('says Copied for two seconds after copying', async () => {
    vi.useFakeTimers({ toFake: ['setTimeout'] })
    stubClipboard(vi.fn().mockResolvedValue(undefined))
    render(
      <Theme>
        <CopyButton value="TNSWU21F5FM6" label="Reference Number" />
      </Theme>,
    )
    const button = screen.getByRole('button', { name: 'Copy Reference Number' })
    fireEvent.focus(button)
    expect(screen.getByRole('tooltip').textContent).toBe('Copy Reference Number')

    fireEvent.click(button)
    await act(() => Promise.resolve())
    expect(screen.getByRole('tooltip').textContent).toBe('Copied')

    act(() => {
      vi.advanceTimersByTime(2000)
    })
    expect(screen.getByRole('tooltip').textContent).toBe('Copy Reference Number')
  })

  it('uses the labels the app set', () => {
    stubClipboard(vi.fn().mockResolvedValue(undefined))
    render(
      <Theme>
        <UiLabelsProvider labels={{ ...defaultUiLabels, copyNamed: (name) => `${name} පිටපත් කරන්න` }}>
          <CopyButton value="TNSWU21F5FM6" label="TNSWU21F5FM6" />
        </UiLabelsProvider>
      </Theme>,
    )

    expect(screen.getByRole('button', { name: 'TNSWU21F5FM6 පිටපත් කරන්න' })).toBeTruthy()
  })

  it('is not shown when the clipboard is unavailable', () => {
    stubClipboard()
    render(
      <Theme>
        <CopyButton value="TNSWU21F5FM6" label="Reference Number" />
      </Theme>,
    )

    expect(screen.queryByRole('button')).toBeNull()
  })

  it('logs a failed copy and keeps the copy label', async () => {
    const error = vi.spyOn(console, 'error').mockImplementation(() => {})
    stubClipboard(vi.fn().mockRejectedValue(new Error('denied')))
    render(
      <Theme>
        <CopyButton value="TNSWU21F5FM6" label="Reference Number" />
      </Theme>,
    )
    const button = screen.getByRole('button', { name: 'Copy Reference Number' })
    fireEvent.focus(button)

    fireEvent.click(button)
    await act(() => Promise.resolve())

    expect(error).toHaveBeenCalled()
    expect(screen.getByRole('tooltip').textContent).toBe('Copy Reference Number')
  })
})
