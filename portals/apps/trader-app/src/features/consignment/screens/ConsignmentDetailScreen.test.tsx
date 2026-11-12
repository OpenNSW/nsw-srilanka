import { Theme } from '@radix-ui/themes'
import { render, screen } from '@testing-library/react'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import { describe, expect, it, vi } from 'vitest'
import { ConsignmentDetailScreen } from '@/features/consignment/screens/ConsignmentDetailScreen'
import type { ConsignmentDetail, FlowOption } from '@/features/consignment/types'

vi.hoisted(() => {
  window.__APP_CONFIG__ = { API_BASE_URL: 'http://api.test' }
})

vi.mock('react-i18next', async (importOriginal) => ({
  ...(await importOriginal<typeof import('react-i18next')>()),
  useTranslation: () => ({ t: (key: string) => key }),
}))

const fee = { templateId: 'fee', label: 'Port fee' }

function renderDetail(startableFlows: FlowOption[], canStartFlows = true) {
  const detail: ConsignmentDetail = {
    id: 'c-1',
    name: 'Test consignment',
    state: 'FINISHED',
    flow: 'EXPORT',
    traderId: 't-1',
    items: [],
    globalContext: {
      consigneeAddress: '',
      consigneeName: '',
      countryOfDestination: '',
      countryOfOrigin: '',
      invoiceDate: '',
      invoiceNumber: '',
    },
    createdAt: '2026-01-01T00:00:00Z',
    updatedAt: '2026-01-01T00:00:00Z',
    workflowNodes: [],
    startableFlows,
  }
  render(
    <Theme>
      <MemoryRouter initialEntries={['/consignments/c-1']}>
        <Routes>
          <Route
            path="/consignments/:consignmentId"
            element={<ConsignmentDetailScreen fetcher={() => Promise.resolve(detail)} canStartFlows={canStartFlows} />}
          />
        </Routes>
      </MemoryRouter>
    </Theme>,
  )
}

describe('ConsignmentDetailScreen flow options', () => {
  it('shows flows returned by the API', async () => {
    renderDetail([fee])
    expect(await screen.findByText('consignments.detail.flows.title')).toBeTruthy()
  })

  it.each([
    { name: 'the API returns no options', flows: [], canStartFlows: true },
    { name: 'the admin view displays the detail', flows: [fee], canStartFlows: false },
  ])('hides the flow controls when $name', async ({ flows, canStartFlows }) => {
    renderDetail(flows, canStartFlows)
    await screen.findByRole('heading', { name: 'Test consignment' })
    expect(screen.queryByText('consignments.detail.flows.title')).toBeNull()
  })
})
