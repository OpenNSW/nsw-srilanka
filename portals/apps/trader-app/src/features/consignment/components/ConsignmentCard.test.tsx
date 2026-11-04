import { Theme } from '@radix-ui/themes'
import { render, screen } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { describe, expect, it, vi } from 'vitest'
import { ConsignmentCard } from '@/features/consignment/components/ConsignmentCard'

// The keys are the text: what each test checks is which label shows, not its wording.
vi.mock('react-i18next', async (importOriginal) => ({
  ...(await importOriginal<typeof import('react-i18next')>()),
  useTranslation: () => ({ t: (key: string) => key }),
}))

type Props = Parameters<typeof ConsignmentCard>[0]

function renderCard(props: Partial<Props> = {}) {
  render(
    <Theme>
      <MemoryRouter>
        <ul>
          <ConsignmentCard to="/consignments/CN-1" id="CN-1" name="Shipment CN-1" state="IN_PROGRESS" {...props} />
        </ul>
      </MemoryRouter>
    </Theme>,
  )
}

describe('ConsignmentCard', () => {
  it('is one link to the consignment', () => {
    renderCard({ flow: 'EXPORT', createdAt: '2026-01-01T00:00:00Z' })

    const link = screen.getByRole('link')
    expect(link.getAttribute('href')).toBe('/consignments/CN-1')
    expect(link.textContent).toContain('Shipment CN-1')
    expect(link.textContent).toContain('CN-1')
  })

  it("shows a consignment's trade flow", () => {
    renderCard({ flow: 'EXPORT' })

    expect(screen.getByText('consignments.tradeFlow.export')).toBeTruthy()
  })

  it('leaves the trade flow out for a case, which has none', () => {
    renderCard({ flow: undefined })

    expect(screen.queryByText('consignments.tradeFlow.export')).toBeNull()
    expect(screen.queryByText('consignments.tradeFlow.import')).toBeNull()
  })

  it('shows a dash when there is no created time', () => {
    renderCard({ createdAt: undefined })

    expect(screen.getByText('-')).toBeTruthy()
  })
})
