import { Theme } from '@radix-ui/themes'
import { act, fireEvent, render, screen } from '@testing-library/react'
import { createMemoryRouter, RouterProvider } from 'react-router-dom'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { AdminConsignmentEngineStatusScreen } from '@/features/admin/screens/AdminConsignmentEngineStatusScreen'
import { getConsignmentForAdmin, getWorkflowEngineStatus } from '@/features/admin/service'
import type { AdminConsignmentDetail, EngineStatus } from '@/features/admin/types'

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
// The debugger needs a signed-in admin.
vi.mock('react-oidc-context', () => ({
  useAuth: () => ({ user: { scope: 'openid consignment:adminread consignment:adminwrite' } }),
}))

const mockGetConsignment = vi.mocked(getConsignmentForAdmin)
const mockGetStatus = vi.mocked(getWorkflowEngineStatus)

const PATTERN = '/admin/consignments/:consignmentId'

function makeConsignment(id: string, workflowIds: string[] = [`wf-${id}`]): AdminConsignmentDetail {
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
    flows: workflowIds.map((workflowId) => ({
      workflowId,
      templateId: `${workflowId}-template`,
      templateName: `Flow ${workflowId}`,
      status: 'STARTED',
      createdAt: '2026-01-01T00:00:00Z',
      updatedAt: '2026-01-01T00:00:00Z',
    })),
  }
}

function makeStatus(workflowId: string): EngineStatus {
  return { workflow_id: workflowId, status: 'RUNNING', nodes: [], audit_trail: [] }
}

function renderScreen(path: string, pattern = PATTERN) {
  const router = createMemoryRouter([{ path: pattern, element: <AdminConsignmentEngineStatusScreen /> }], {
    initialEntries: [path],
  })
  render(
    <Theme>
      <RouterProvider router={router} />
    </Theme>,
  )
  return router
}

describe('AdminConsignmentEngineStatusScreen', () => {
  afterEach(() => {
    vi.resetAllMocks()
  })

  it('shows the consignment and its first flow', async () => {
    mockGetConsignment.mockResolvedValue(makeConsignment('c-1'))
    mockGetStatus.mockResolvedValue(makeStatus('wf-c-1'))

    renderScreen('/admin/consignments/c-1')

    expect(screen.getByText('Loading consignment…')).toBeTruthy()
    expect(await screen.findByText('Shipment c-1')).toBeTruthy()
    expect(await screen.findByRole('heading', { name: 'Flow wf-c-1' })).toBeTruthy()
    expect(mockGetConsignment).toHaveBeenCalledWith('c-1', expect.any(AbortSignal))
    expect(mockGetStatus).toHaveBeenCalledWith('wf-c-1', expect.any(AbortSignal))
  })

  it('asks for an ID when the route has none', () => {
    renderScreen('/admin/consignments', '/admin/consignments/:consignmentId?')

    expect(screen.getByText('A consignment ID is required.')).toBeTruthy()
    expect(mockGetConsignment).not.toHaveBeenCalled()
  })

  it('tells "not found" apart from "failed to load"', async () => {
    mockGetConsignment.mockResolvedValueOnce(null)
    renderScreen('/admin/consignments/missing')
    expect(await screen.findByText('Consignment not found')).toBeTruthy()
    expect(screen.queryByText('Failed to load consignment')).toBeNull()
  })

  it('shows a failed load and loads again on Try Again', async () => {
    mockGetConsignment.mockRejectedValueOnce(new Error('network down'))
    mockGetConsignment.mockResolvedValueOnce(makeConsignment('c-1'))
    mockGetStatus.mockResolvedValue(makeStatus('wf-c-1'))

    renderScreen('/admin/consignments/c-1')
    expect(await screen.findByText('Failed to load consignment')).toBeTruthy()
    expect(screen.queryByText('Consignment not found')).toBeNull()

    fireEvent.click(screen.getByRole('button', { name: 'Try Again' }))

    expect(await screen.findByText('Shipment c-1')).toBeTruthy()
    expect(mockGetConsignment).toHaveBeenCalledTimes(2)
  })

  it("shows a spinner, not the last consignment's data, while the next one loads", async () => {
    mockGetConsignment.mockResolvedValueOnce(makeConsignment('c-1'))
    mockGetConsignment.mockReturnValueOnce(new Promise(() => undefined))
    mockGetStatus.mockResolvedValue(makeStatus('wf-c-1'))

    const router = renderScreen('/admin/consignments/c-1')
    expect(await screen.findByText('Shipment c-1')).toBeTruthy()

    await act(() => router.navigate('/admin/consignments/c-2'))

    expect(screen.getByText('Loading consignment…')).toBeTruthy()
    expect(screen.queryByText('Shipment c-1')).toBeNull()
    expect(mockGetConsignment).toHaveBeenLastCalledWith('c-2', expect.any(AbortSignal))
  })

  it('opens the flow named in ?flow=, and the first flow for an ID that is not one of them', async () => {
    mockGetConsignment.mockResolvedValue(makeConsignment('c-1', ['wf-a', 'wf-b']))
    mockGetStatus.mockImplementation((workflowId) => Promise.resolve(makeStatus(workflowId)))

    const router = renderScreen('/admin/consignments/c-1?flow=wf-b')
    expect(await screen.findByRole('heading', { name: 'Flow wf-b' })).toBeTruthy()

    await act(() => router.navigate('/admin/consignments/c-1?flow=someone-elses'))
    expect(await screen.findByRole('heading', { name: 'Flow wf-a' })).toBeTruthy()
  })

  it('reloads the consignment as well when the engine status is refreshed', async () => {
    mockGetConsignment.mockResolvedValue(makeConsignment('c-1'))
    mockGetStatus.mockResolvedValue(makeStatus('wf-c-1'))

    renderScreen('/admin/consignments/c-1')
    expect(await screen.findByRole('heading', { name: 'Flow wf-c-1' })).toBeTruthy()

    fireEvent.click(screen.getByRole('button', { name: 'Refresh' }))

    await vi.waitFor(() => expect(mockGetStatus).toHaveBeenCalledTimes(2))
    expect(mockGetConsignment).toHaveBeenCalledTimes(2)
    expect(screen.getByText('Shipment c-1')).toBeTruthy()
  })
})
