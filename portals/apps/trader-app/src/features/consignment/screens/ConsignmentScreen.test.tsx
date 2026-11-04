import { Theme } from '@radix-ui/themes'
import { fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { ConsignmentScreen } from '@/features/consignment/screens/ConsignmentScreen'
import { getAllConsignments } from '@/features/consignment/service'
import type { ConsignmentListResult, ConsignmentSummary } from '@/features/consignment/types'

// config.js sets this in the app; @/constants reads it as it loads.
vi.hoisted(() => {
  window.__APP_CONFIG__ = { API_BASE_URL: 'http://api.test' }
})

vi.mock('@/features/consignment/service', () => ({ getAllConsignments: vi.fn(), createConsignment: vi.fn() }))
// The list needs a signed-in user granted to read consignments.
vi.mock('react-oidc-context', () => ({ useAuth: () => ({ user: { scope: 'openid consignment:read' } }) }))
// The keys are the text: what each test checks is which message shows, not its wording.
vi.mock('react-i18next', async (importOriginal) => ({
  ...(await importOriginal<typeof import('react-i18next')>()),
  useTranslation: () => ({ t: (key: string) => key }),
}))

const mockGetAll = vi.mocked(getAllConsignments)

function makeConsignment(id: string): ConsignmentSummary {
  return {
    id,
    name: `Shipment ${id}`,
    flow: 'EXPORT',
    traderId: 't-1',
    callerRoles: [],
    state: 'IN_PROGRESS',
    items: [],
    createdAt: '2026-01-01T00:00:00Z',
    updatedAt: '2026-01-01T00:00:00Z',
    workflowNodeCount: 0,
    completedWorkflowNodeCount: 0,
  }
}

// One page of a 120-consignment list, so there is always a next page.
function page(offset: number, ids: string[]): ConsignmentListResult {
  return { items: ids.map(makeConsignment), total: 120, offset, limit: 50 }
}

function renderScreen() {
  render(
    <Theme>
      <MemoryRouter>
        <ConsignmentScreen />
      </MemoryRouter>
    </Theme>,
  )
}

// The offset of the latest list request.
function lastOffset() {
  return mockGetAll.mock.lastCall?.[0]
}

// jsdom applies no CSS, so each item renders both as a card (phones) and as a
// table row (md: up); the tests find an item by its text with the *All queries.
describe('ConsignmentScreen', () => {
  afterEach(() => {
    mockGetAll.mockReset()
  })

  it('loads the first page with no filters', async () => {
    mockGetAll.mockResolvedValue(page(0, ['CN-1']))
    renderScreen()

    expect(await screen.findAllByText('Shipment CN-1')).toBeTruthy()
    expect(mockGetAll).toHaveBeenCalledWith(0, 50, 'all', undefined, '', expect.any(AbortSignal))
  })

  it('shows each item as a card on a phone and as a table row from md: up, both opening it', async () => {
    mockGetAll.mockResolvedValue(page(0, ['CN-1']))
    renderScreen()
    await screen.findAllByText('Shipment CN-1')

    const card = screen.getByRole('listitem')
    expect(card.closest('ul')?.className).toContain('md:hidden')
    expect(within(card).getByRole('link').getAttribute('href')).toBe('/consignments/CN-1')

    const table = screen.getByRole('table')
    expect(table.closest('.hidden')?.className).toContain('md:block')
    expect(
      within(table)
        .getByRole('link', { name: /Shipment CN-1/ })
        .getAttribute('href'),
    ).toBe('/consignments/CN-1')
  })

  it('shows a failed load as an error, not an empty list, and retries it', async () => {
    mockGetAll.mockRejectedValueOnce(new Error('offline')).mockResolvedValue(page(0, ['CN-1']))
    renderScreen()

    expect(await screen.findByText('consignments.list.error.loadFailed')).toBeTruthy()
    expect(screen.queryByText('consignments.list.empty.none')).toBeNull()

    fireEvent.click(screen.getByRole('button', { name: 'Try Again' }))
    expect(await screen.findAllByText('Shipment CN-1')).toBeTruthy()
    expect(screen.queryByText('consignments.list.error.loadFailed')).toBeNull()
  })

  it("drops the previous page's rows and count when the next page fails", async () => {
    mockGetAll.mockResolvedValueOnce(page(0, ['CN-1'])).mockRejectedValue(new Error('offline'))
    renderScreen()
    await screen.findAllByText('Shipment CN-1')

    fireEvent.click(screen.getByRole('button', { name: 'Next' }))

    expect(await screen.findByText('consignments.list.error.loadFailed')).toBeTruthy()
    expect(screen.queryAllByText('Shipment CN-1')).toHaveLength(0)
    expect(screen.queryByText('120')).toBeNull()
    expect(screen.queryByText('Total: 120')).toBeNull()
  })

  it('keeps the current rows under the loading overlay while the next page loads', async () => {
    let resolveNext: (result: ConsignmentListResult) => void = () => {}
    mockGetAll
      .mockResolvedValueOnce(page(0, ['CN-1']))
      .mockReturnValueOnce(new Promise((resolve) => (resolveNext = resolve)))
    renderScreen()
    await screen.findAllByText('Shipment CN-1')

    fireEvent.click(screen.getByRole('button', { name: 'Next' }))

    await waitFor(() => expect(lastOffset()).toBe(50))
    expect(within(screen.getByRole('status')).getByText('consignments.list.loading')).toBeTruthy()
    expect(screen.getAllByText('Shipment CN-1')).not.toHaveLength(0)

    resolveNext(page(50, ['CN-51']))
    expect(await screen.findAllByText('Shipment CN-51')).toBeTruthy()
    expect(screen.queryAllByText('Shipment CN-1')).toHaveLength(0)
  })

  it('returns to the first page when the search changes', async () => {
    mockGetAll.mockImplementation((offset = 0) => Promise.resolve(page(offset, [`CN-${offset + 1}`])))
    renderScreen()
    await screen.findAllByText('Shipment CN-1')
    fireEvent.click(screen.getByRole('button', { name: 'Next' }))
    await screen.findAllByText('Shipment CN-51')

    fireEvent.change(screen.getByPlaceholderText('consignments.list.searchPlaceholder'), {
      target: { value: 'tea' },
    })

    // The search is debounced, so the request waits for typing to pause.
    await waitFor(() =>
      expect(mockGetAll).toHaveBeenLastCalledWith(0, 50, 'all', undefined, 'tea', expect.any(AbortSignal)),
    )
  })

  it('returns to the first page when a filter changes', async () => {
    mockGetAll.mockImplementation((offset = 0) => Promise.resolve(page(offset, [`CN-${offset + 1}`])))
    renderScreen()
    await screen.findAllByText('Shipment CN-1')
    fireEvent.click(screen.getByRole('button', { name: 'Next' }))
    await screen.findAllByText('Shipment CN-51')

    const [stateFilter] = screen.getAllByRole('combobox')
    fireEvent.keyDown(stateFilter, { key: 'Enter' })
    fireEvent.keyDown(await screen.findByRole('option', { name: 'consignments.list.filter.finished' }), {
      key: 'Enter',
    })

    await waitFor(() =>
      expect(mockGetAll).toHaveBeenLastCalledWith(0, 50, 'FINISHED', undefined, '', expect.any(AbortSignal)),
    )
  })
})
