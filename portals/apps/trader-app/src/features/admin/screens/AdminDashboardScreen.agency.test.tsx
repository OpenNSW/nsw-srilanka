import { Theme } from '@radix-ui/themes'
import { fireEvent, render, screen } from '@testing-library/react'
import { createMemoryRouter, RouterProvider } from 'react-router-dom'
import { describe, expect, it, vi } from 'vitest'
import { AdminDashboardScreen } from '@/features/admin/screens/AdminDashboardScreen'

// An agency deployment searches cases. isAgencyMode is read once as the screen module loads, so
// this case gets a file of its own.
vi.mock('@/features/case/mode', () => ({ isAgencyMode: true }))

// config.js sets this in the app; @/constants reads it as it loads.
vi.hoisted(() => {
  window.__APP_CONFIG__ = { API_BASE_URL: 'http://api.test' }
})

function renderScreen() {
  const router = createMemoryRouter(
    [
      { path: '/admin', element: <AdminDashboardScreen /> },
      { path: '/admin/cases/:consignmentId', element: <p>Debugger</p> },
    ],
    { initialEntries: ['/admin'] },
  )
  render(
    <Theme>
      <RouterProvider router={router} />
    </Theme>,
  )
  return router
}

describe('AdminDashboardScreen in an agency', () => {
  it('shows the case search', () => {
    renderScreen()
    expect(screen.getByRole('heading', { name: 'Case search' })).toBeTruthy()
    expect(screen.getByText('Open a case you have been referred to.')).toBeTruthy()
    expect(screen.getByLabelText('Case ID')).toBeTruthy()
    expect(screen.getByText(/shown on the case list and detail screens/)).toBeTruthy()
  })

  it('opens the case debugger for the trimmed ID', async () => {
    const router = renderScreen()
    fireEvent.change(screen.getByLabelText('Case ID'), { target: { value: '  abc-123  ' } })
    fireEvent.click(screen.getByRole('button', { name: 'Search' }))
    expect(await screen.findByText('Debugger')).toBeTruthy()
    expect(router.state.location.pathname).toBe('/admin/cases/abc-123')
  })
})
