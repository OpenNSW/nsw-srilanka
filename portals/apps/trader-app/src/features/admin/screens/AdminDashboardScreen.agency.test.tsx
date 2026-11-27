import { Theme } from '@radix-ui/themes'
import { fireEvent, render, screen } from '@testing-library/react'
import { createMemoryRouter, RouterProvider } from 'react-router-dom'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { AdminDashboardScreen } from '@/features/admin/screens/AdminDashboardScreen'
import { getCaseForAdmin, getConsignmentForAdmin } from '@/features/admin/service'
import type { AdminConsignmentDetail } from '@/features/admin/types'

// An agency deployment searches cases. isAgencyMode is read once as the screen module loads, so
// this case gets a file of its own.
vi.mock('@/features/case/mode', () => ({ isAgencyMode: true }))
vi.mock('@/features/admin/service', () => ({
  getConsignmentForAdmin: vi.fn(),
  getCaseForAdmin: vi.fn(),
  getWorkflowEngineStatus: vi.fn(),
  getTaskWorkflowEngineStatus: vi.fn(),
  forceParkNode: vi.fn(),
  resolveAdminIntervention: vi.fn(),
}))

// config.js sets this in the app; @/constants reads it as it loads.
vi.hoisted(() => {
  window.__APP_CONFIG__ = { API_BASE_URL: 'http://api.test' }
})

const mockGetCase = vi.mocked(getCaseForAdmin)

function makeCase(id: string): AdminConsignmentDetail {
  return {
    id,
    name: `Case ${id}`,
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

function search(id: string) {
  fireEvent.change(screen.getByLabelText('Case ID'), { target: { value: id } })
  fireEvent.click(screen.getByRole('button', { name: /Search/ }))
}

afterEach(() => {
  vi.clearAllMocks()
})

describe('AdminDashboardScreen in an agency', () => {
  it('shows the case search', () => {
    renderScreen()
    expect(screen.getByRole('heading', { name: 'Case search' })).toBeTruthy()
    expect(screen.getByText('Open a case you have been referred to.')).toBeTruthy()
    expect(screen.getByLabelText('Case ID')).toBeTruthy()
    expect(screen.getByText(/shown on the case list and detail screens/)).toBeTruthy()
  })

  it('looks the case up through the admin cases endpoint and opens its debugger', async () => {
    mockGetCase.mockResolvedValue(makeCase('abc-123'))
    const router = renderScreen()
    search('  abc-123  ')
    expect(await screen.findByText('Debugger')).toBeTruthy()
    expect(router.state.location.pathname).toBe('/admin/cases/abc-123')
    expect(mockGetCase).toHaveBeenCalledWith('abc-123')
    expect(getConsignmentForAdmin).not.toHaveBeenCalled()
  })

  it('reports a missing case', async () => {
    mockGetCase.mockResolvedValue(null)
    const router = renderScreen()
    search('missing-1')
    expect(await screen.findByText('Case not found. Check the ID and try again.')).toBeTruthy()
    expect(router.state.location.pathname).toBe('/admin')
  })

  it('reports a failed lookup in case wording', async () => {
    mockGetCase.mockRejectedValue(new Error('network down'))
    renderScreen()
    search('abc-123')
    expect(await screen.findByText("Couldn't look up the case. Please try again.")).toBeTruthy()
  })
})
