import { Theme } from '@radix-ui/themes'
import { render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { AdminDashboardScreen } from '@/features/admin/screens/AdminDashboardScreen'

// An agency deployment searches cases. isAgencyMode is read once as the screen module loads, so
// this case gets a file of its own.
vi.mock('@/features/case/mode', () => ({ isAgencyMode: true }))

// config.js sets this in the app; @/constants reads it as it loads.
vi.hoisted(() => {
  window.__APP_CONFIG__ = { API_BASE_URL: 'http://api.test' }
})

describe('AdminDashboardScreen in an agency', () => {
  it('shows the case search', () => {
    render(
      <Theme>
        <AdminDashboardScreen />
      </Theme>,
    )
    expect(screen.getByRole('heading', { name: 'Case search' })).toBeTruthy()
    expect(screen.getByText('Open a case you have been referred to.')).toBeTruthy()
    expect(screen.getByLabelText('Case ID')).toBeTruthy()
    expect(screen.getByText(/shown on the case list and detail screens/)).toBeTruthy()
  })
})
