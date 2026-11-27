import { Theme } from '@radix-ui/themes'
import { fireEvent, render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { AdminDashboardScreen } from '@/features/admin/screens/AdminDashboardScreen'

// config.js sets this in the app; @/constants reads it as it loads.
vi.hoisted(() => {
  window.__APP_CONFIG__ = { API_BASE_URL: 'http://api.test' }
})

function renderScreen() {
  render(
    <Theme>
      <AdminDashboardScreen />
    </Theme>,
  )
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
    const search = screen.getByRole('button', { name: 'Search' })
    expect((search as HTMLButtonElement).disabled).toBe(true)

    fireEvent.change(input, { target: { value: '   ' } })
    expect((search as HTMLButtonElement).disabled).toBe(true)

    fireEvent.change(input, { target: { value: ' abc-123 ' } })
    expect((search as HTMLButtonElement).disabled).toBe(false)
  })
})
