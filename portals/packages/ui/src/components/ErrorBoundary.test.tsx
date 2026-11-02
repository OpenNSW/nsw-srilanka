import { useState } from 'react'
import { fireEvent, render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { ErrorBoundary } from './ErrorBoundary'

function Bomb({ explode }: { explode: boolean }) {
  if (explode) throw new Error('boom')
  return <p>fine</p>
}

describe('ErrorBoundary', () => {
  it('shows the fallback with the error, reports it, and renders children again on reset', () => {
    // React logs a caught render error to the console; keep the test output clean.
    vi.spyOn(console, 'error').mockImplementation(() => {})
    const onError = vi.fn()

    function Harness() {
      const [explode, setExplode] = useState(true)
      return (
        <ErrorBoundary
          onError={onError}
          fallback={(error, reset) => (
            <button
              onClick={() => {
                setExplode(false)
                reset()
              }}
            >
              {error.message}
            </button>
          )}
        >
          <Bomb explode={explode} />
        </ErrorBoundary>
      )
    }
    render(<Harness />)

    expect(onError).toHaveBeenCalledWith(expect.objectContaining({ message: 'boom' }), expect.anything())
    fireEvent.click(screen.getByRole('button', { name: 'boom' }))
    expect(screen.getByText('fine')).toBeTruthy()
  })
})
