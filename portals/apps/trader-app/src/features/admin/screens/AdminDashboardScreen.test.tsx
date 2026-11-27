import { Theme } from '@radix-ui/themes'
import { act, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { createMemoryRouter, RouterProvider } from 'react-router-dom'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { AdminDashboardScreen } from '@/features/admin/screens/AdminDashboardScreen'
import { getCaseForAdmin, getConsignmentForAdmin } from '@/features/admin/service'
import type { AdminConsignmentDetail } from '@/features/admin/types'

// config.js sets this in the app; @/constants reads it as it loads.
vi.hoisted(() => {
  window.__APP_CONFIG__ = { API_BASE_URL: 'http://api.test' }
})

vi.mock('@/features/admin/service', () => ({
  getConsignmentForAdmin: vi.fn(),
  getCaseForAdmin: vi.fn(),
  getWorkflowEngineStatus: vi.fn(),
  getTaskWorkflowEngineStatus: vi.fn(),
  forceParkNode: vi.fn(),
  resolveAdminIntervention: vi.fn(),
}))

const mockGetConsignment = vi.mocked(getConsignmentForAdmin)

const NOT_FOUND = 'Consignment not found. Check the ID with the trader.'
const ERROR = "Couldn't look up the consignment. Please try again."

function makeConsignment(id: string): AdminConsignmentDetail {
  return {
    id,
    name: `Shipment ${id}`,
    flow: 'EXPORT',
    traderId: 't-1',
    state: 'IN_PROGRESS',
    items: [],
    globalContext: {} as AdminConsignmentDetail['globalContext'],
    createdAt: '2026-01-01T00:00:00Z',
    updatedAt: '2026-01-01T00:00:00Z',
    workflowNodes: [],
    flows: [],
  }
}

// A lookup the test settles itself, to see the screen while it is pending.
function deferred<T>() {
  let resolve!: (value: T) => void
  const promise = new Promise<T>((r) => {
    resolve = r
  })
  return { promise, resolve }
}

// The debugger route stands in for AdminConsignmentEngineStatusScreen.
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

function input() {
  return screen.getByLabelText<HTMLInputElement>('Consignment ID')
}

function searchButton() {
  return screen.getByRole<HTMLButtonElement>('button', { name: /Search/ })
}

function search(id: string) {
  fireEvent.change(input(), { target: { value: id } })
  fireEvent.click(searchButton())
}

afterEach(() => {
  vi.clearAllMocks()
})

describe('AdminDashboardScreen', () => {
  it('shows the consignment search', () => {
    renderScreen()
    expect(screen.getByRole('heading', { name: 'Consignment search' })).toBeTruthy()
    expect(screen.getByText('Open a consignment a trader has referred you to.')).toBeTruthy()
    expect(document.activeElement).toBe(input())
  })

  it('disables Search until an ID is typed, ignoring spaces', () => {
    renderScreen()
    expect(searchButton().disabled).toBe(true)

    fireEvent.change(input(), { target: { value: '   ' } })
    expect(searchButton().disabled).toBe(true)

    fireEvent.change(input(), { target: { value: ' abc-123 ' } })
    expect(searchButton().disabled).toBe(false)
  })

  it('looks up the trimmed ID and opens the debugger when it exists', async () => {
    mockGetConsignment.mockResolvedValue(makeConsignment('abc-123'))
    const router = renderScreen()
    search('  abc-123  ')
    expect(await screen.findByText('Debugger')).toBeTruthy()
    expect(router.state.location.pathname).toBe('/admin/consignments/abc-123')
    expect(mockGetConsignment).toHaveBeenCalledWith('abc-123')
    expect(getCaseForAdmin).not.toHaveBeenCalled()
  })

  it('submits on Enter', async () => {
    mockGetConsignment.mockResolvedValue(makeConsignment('abc-123'))
    const router = renderScreen()
    fireEvent.change(input(), { target: { value: 'abc-123' } })
    fireEvent.submit(input().closest('form')!)
    expect(await screen.findByText('Debugger')).toBeTruthy()
    expect(router.state.location.pathname).toBe('/admin/consignments/abc-123')
  })

  it('encodes the ID so it stays one path segment', async () => {
    mockGetConsignment.mockResolvedValue(makeConsignment('a/b?c'))
    const router = renderScreen()
    search('a/b?c')
    expect(await screen.findByText('Debugger')).toBeTruthy()
    expect(router.state.location.pathname).toBe('/admin/consignments/a%2Fb%3Fc')
  })

  it("doesn't look anything up when only spaces are submitted", () => {
    const router = renderScreen()
    fireEvent.change(input(), { target: { value: '   ' } })
    fireEvent.submit(input().closest('form')!)
    expect(mockGetConsignment).not.toHaveBeenCalled()
    expect(router.state.location.pathname).toBe('/admin')
  })

  it('reports a missing consignment and keeps the ID', async () => {
    mockGetConsignment.mockResolvedValue(null)
    const router = renderScreen()
    search('missing-1')
    expect((await screen.findByRole('alert')).textContent).toContain(NOT_FOUND)
    expect(router.state.location.pathname).toBe('/admin')
    expect(input().value).toBe('missing-1')
    expect(input().getAttribute('aria-invalid')).toBe('true')
  })

  it('clears the not-found message once the ID is edited', async () => {
    mockGetConsignment.mockResolvedValue(null)
    renderScreen()
    search('missing-1')
    expect(await screen.findByText(NOT_FOUND)).toBeTruthy()

    fireEvent.change(input(), { target: { value: 'missing-12' } })
    expect(screen.queryByText(NOT_FOUND)).toBeNull()
    expect(input().getAttribute('aria-invalid')).toBeNull()
  })

  it('reports a failed lookup, and Retry looks the same ID up again', async () => {
    mockGetConsignment.mockRejectedValueOnce(new Error('network down'))
    mockGetConsignment.mockResolvedValueOnce(makeConsignment('abc-123'))
    const router = renderScreen()
    search('abc-123')
    expect(await screen.findByText(ERROR)).toBeTruthy()
    expect(router.state.location.pathname).toBe('/admin')

    fireEvent.click(screen.getByRole('button', { name: 'Retry' }))
    expect(await screen.findByText('Debugger')).toBeTruthy()
    expect(mockGetConsignment).toHaveBeenCalledTimes(2)
    expect(mockGetConsignment).toHaveBeenLastCalledWith('abc-123')
    expect(router.state.location.pathname).toBe('/admin/consignments/abc-123')
  })

  it('shows the lookup as busy and ignores a second submit while it runs', async () => {
    const pending = deferred<AdminConsignmentDetail | null>()
    mockGetConsignment.mockReturnValue(pending.promise)
    renderScreen()
    search('abc-123')

    await waitFor(() => expect(searchButton().disabled).toBe(true))
    expect(input().readOnly).toBe(true)
    fireEvent.submit(input().closest('form')!)
    expect(mockGetConsignment).toHaveBeenCalledTimes(1)

    await act(async () => {
      pending.resolve(null)
      await pending.promise
    })
    expect(await screen.findByText(NOT_FOUND)).toBeTruthy()
    expect(input().readOnly).toBe(false)
  })

  it('ignores a lookup that settles after the screen has gone', async () => {
    const pending = deferred<AdminConsignmentDetail | null>()
    mockGetConsignment.mockReturnValue(pending.promise)
    const router = renderScreen()
    search('abc-123')
    await waitFor(() => expect(searchButton().disabled).toBe(true))

    await act(async () => {
      await router.navigate('/admin/consignments/other')
    })
    await act(async () => {
      pending.resolve(makeConsignment('abc-123'))
      await pending.promise
    })
    expect(router.state.location.pathname).toBe('/admin/consignments/other')
  })
})
