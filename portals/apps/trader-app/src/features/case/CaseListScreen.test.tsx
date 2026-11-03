import { Theme } from '@radix-ui/themes'
import { fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { CaseListScreen } from '@/features/case/CaseListScreen'
import { getCases } from '@/features/case/service'
import type { CaseListResult } from '@/features/case/types'

// config.js sets this in the app; @/constants reads it as it loads.
vi.hoisted(() => {
  window.__APP_CONFIG__ = { API_BASE_URL: 'http://api.test' }
})

vi.mock('@/features/case/service', () => ({ getCases: vi.fn() }))
// The keys are the text: what each test checks is which message shows, not its wording.
vi.mock('react-i18next', async (importOriginal) => ({
  ...(await importOriginal<typeof import('react-i18next')>()),
  useTranslation: () => ({ t: (key: string) => key }),
}))

const mockGetCases = vi.mocked(getCases)

// One page of a 120-case list, so there is always a next page.
function page(offset: number, ids: string[]): CaseListResult {
  return {
    items: ids.map((id) => ({
      id,
      name: `Case ${id}`,
      state: 'IN_PROGRESS',
      createdAt: '2026-01-01T00:00:00Z',
      updatedAt: '2026-01-01T00:00:00Z',
    })),
    total: 120,
    offset,
    limit: 50,
  }
}

function renderScreen() {
  render(
    <Theme>
      <MemoryRouter>
        <CaseListScreen />
      </MemoryRouter>
    </Theme>,
  )
}

describe('CaseListScreen', () => {
  afterEach(() => {
    mockGetCases.mockReset()
  })

  it('loads the first page', async () => {
    mockGetCases.mockResolvedValue(page(0, ['CS-1']))
    renderScreen()

    expect(await screen.findByText('Case CS-1')).toBeTruthy()
    expect(mockGetCases).toHaveBeenCalledWith(0, 50, expect.any(AbortSignal))
  })

  it('shows a failed load as an error, not an empty list, and retries it', async () => {
    mockGetCases.mockRejectedValueOnce(new Error('offline')).mockResolvedValue(page(0, ['CS-1']))
    renderScreen()

    expect(await screen.findByText('cases.list.error')).toBeTruthy()
    expect(screen.queryByText('cases.list.empty')).toBeNull()

    fireEvent.click(screen.getByRole('button', { name: 'Try Again' }))
    expect(await screen.findByText('Case CS-1')).toBeTruthy()
    expect(screen.queryByText('cases.list.error')).toBeNull()
  })

  it("drops the previous page's rows and count when the next page fails", async () => {
    mockGetCases.mockResolvedValueOnce(page(0, ['CS-1'])).mockRejectedValue(new Error('offline'))
    renderScreen()
    await screen.findByText('Case CS-1')

    fireEvent.click(screen.getByRole('button', { name: 'Next' }))

    expect(await screen.findByText('cases.list.error')).toBeTruthy()
    expect(screen.queryByText('Case CS-1')).toBeNull()
    expect(screen.queryByText('120')).toBeNull()
    expect(screen.queryByText('Total: 120')).toBeNull()
  })

  it('keeps the current rows under the loading overlay while the next page loads', async () => {
    let resolveNext: (result: CaseListResult) => void = () => {}
    mockGetCases
      .mockResolvedValueOnce(page(0, ['CS-1']))
      .mockReturnValueOnce(new Promise((resolve) => (resolveNext = resolve)))
    renderScreen()
    await screen.findByText('Case CS-1')

    fireEvent.click(screen.getByRole('button', { name: 'Next' }))

    await waitFor(() => expect(mockGetCases.mock.lastCall?.[0]).toBe(50))
    expect(within(screen.getByRole('status')).getByText('cases.list.loading')).toBeTruthy()
    expect(screen.getByText('Case CS-1')).toBeTruthy()

    resolveNext(page(50, ['CS-51']))
    expect(await screen.findByText('Case CS-51')).toBeTruthy()
    expect(screen.queryByText('Case CS-1')).toBeNull()
  })
})
