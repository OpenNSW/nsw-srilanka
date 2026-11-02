import { fireEvent, render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { defaultUiLabels, UiLabelsProvider } from '../labels'
import { PageState } from './PageState'

describe('PageState', () => {
  it('announces a loading state', () => {
    render(<PageState variant="loading" title="Loading consignment…" />)

    expect(screen.getByRole('status').textContent).toContain('Loading consignment…')
  })

  it('shows an empty state with its description and actions', () => {
    render(
      <PageState
        variant="empty"
        title="No workflow yet"
        description="Steps appear as they start."
        actions={<button>Back</button>}
      />,
    )

    expect(screen.getByText('No workflow yet')).toBeTruthy()
    expect(screen.getByText('Steps appear as they start.')).toBeTruthy()
    expect(screen.getByRole('button', { name: 'Back' })).toBeTruthy()
    expect(screen.queryByRole('status')).toBeNull()
  })

  it('offers Try Again on an error, in the labels the app set, after its other actions', () => {
    const onRetry = vi.fn()
    render(
      <UiLabelsProvider labels={{ ...defaultUiLabels, tryAgain: 'නැවත උත්සාහ කරන්න' }}>
        <PageState variant="error" title="Failed to load" onRetry={onRetry} actions={<button>Back</button>} />
      </UiLabelsProvider>,
    )

    const buttons = screen.getAllByRole('button').map((button) => button.textContent)
    expect(buttons).toEqual(['Back', 'නැවත උත්සාහ කරන්න'])
    fireEvent.click(screen.getByRole('button', { name: 'නැවත උත්සාහ කරන්න' }))
    expect(onRetry).toHaveBeenCalledOnce()
  })

  it('has no buttons on an error without onRetry or actions', () => {
    render(<PageState variant="error" title="Not found" />)

    expect(screen.queryByRole('button')).toBeNull()
  })
})
