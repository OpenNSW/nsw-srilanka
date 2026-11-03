import { render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { Banner } from './Banner'

describe('Banner', () => {
  it('announces an error, with its title and message', () => {
    render(
      <Banner variant="error" title="Sign-in failed">
        The session expired.
      </Banner>,
    )

    const banner = screen.getByRole('alert')
    expect(banner.textContent).toContain('Sign-in failed')
    expect(banner.textContent).toContain('The session expired.')
    expect(banner.className).toContain('bg-error-subtle')
  })

  it('does not interrupt for the other variants', () => {
    render(<Banner variant="warning">This task is on hold.</Banner>)

    expect(screen.queryByRole('alert')).toBeNull()
    expect(screen.getByText('This task is on hold.').closest('div[class*="bg-warning-subtle"]')).toBeTruthy()
  })
})
