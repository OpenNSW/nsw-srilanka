import { Theme } from '@radix-ui/themes'
import { fireEvent, render, screen } from '@testing-library/react'
import { createMemoryRouter, RouterProvider } from 'react-router-dom'
import { describe, expect, it, vi } from 'vitest'
import { AdminDashboardScreen } from '@/features/admin/screens/AdminDashboardScreen'

// config.js sets this in the app; @/constants reads it as it loads.
vi.hoisted(() => {
  window.__APP_CONFIG__ = { API_BASE_URL: 'http://api.test' }
})

// The debugger route stands in for AdminConsignmentEngineStatusScreen, showing the ID it got.
function renderScreen() {
  const router = createMemoryRouter(
    [
      { path: '/admin', element: <AdminDashboardScreen /> },
      { path: '/admin/consignments/:consignmentId', element: <p>Debugger</p> },
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

function search(id: string) {
  fireEvent.change(screen.getByLabelText('Consignment ID'), { target: { value: id } })
  fireEvent.click(screen.getByRole('button', { name: 'Search' }))
}

describe('AdminDashboardScreen', () => {
  it('shows the consignment search', () => {
    renderScreen()
    expect(screen.getByRole('heading', { name: 'Consignment search' })).toBeTruthy()
    expect(screen.getByText('Open a consignment a trader has referred you to.')).toBeTruthy()
    expect(document.activeElement).toBe(screen.getByLabelText('Consignment ID'))
  })

  it('disables Search until an ID is typed, ignoring spaces', () => {
    renderScreen()
    const input = screen.getByLabelText('Consignment ID')
    const button = screen.getByRole('button', { name: 'Search' })
    expect((button as HTMLButtonElement).disabled).toBe(true)

    fireEvent.change(input, { target: { value: '   ' } })
    expect((button as HTMLButtonElement).disabled).toBe(true)

    fireEvent.change(input, { target: { value: ' abc-123 ' } })
    expect((button as HTMLButtonElement).disabled).toBe(false)
  })

  it('opens the debugger for the trimmed ID', async () => {
    const router = renderScreen()
    search('  abc-123  ')
    expect(await screen.findByText('Debugger')).toBeTruthy()
    expect(router.state.location.pathname).toBe('/admin/consignments/abc-123')
  })

  it('submits on Enter', async () => {
    const router = renderScreen()
    const input = screen.getByLabelText('Consignment ID')
    fireEvent.change(input, { target: { value: 'abc-123' } })
    fireEvent.submit(input.closest('form')!)
    expect(await screen.findByText('Debugger')).toBeTruthy()
    expect(router.state.location.pathname).toBe('/admin/consignments/abc-123')
  })

  it('encodes the ID so it stays one path segment', async () => {
    const router = renderScreen()
    search('a/b?c')
    expect(await screen.findByText('Debugger')).toBeTruthy()
    expect(router.state.location.pathname).toBe('/admin/consignments/a%2Fb%3Fc')
  })

  it("doesn't navigate when only spaces are submitted", () => {
    const router = renderScreen()
    const input = screen.getByLabelText('Consignment ID')
    fireEvent.change(input, { target: { value: '   ' } })
    fireEvent.submit(input.closest('form')!)
    expect(router.state.location.pathname).toBe('/admin')
  })
})
