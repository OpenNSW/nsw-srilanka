import { Theme } from '@radix-ui/themes'
import { fireEvent, render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { AuxiliaryServices } from '@/features/consignment/components/AuxiliaryServices'
import type { FlowOption } from '@/features/consignment/types'

// The keys are the text: what each test checks is which control shows, not its wording.
vi.mock('react-i18next', async (importOriginal) => ({
  ...(await importOriginal<typeof import('react-i18next')>()),
  useTranslation: () => ({ t: (key: string) => key }),
}))

const flows: FlowOption[] = [
  { templateId: 'slpa-sundry-payment', label: 'SLPA sundry payment' },
  { templateId: 'customs-additional-cdn', label: 'Add a Cargo Dispatch Note' },
]

function renderServices(list = flows) {
  render(
    <Theme>
      <AuxiliaryServices flows={list} onStart={vi.fn().mockResolvedValue(undefined)} />
    </Theme>,
  )
}

describe('AuxiliaryServices', () => {
  it('starts open, with a card per service', () => {
    renderServices()

    const toggle = screen.getByRole('button', { expanded: true })
    expect(toggle.getAttribute('aria-controls')).toBe('auxiliary-services-list')
    expect(screen.getByText('SLPA sundry payment')).toBeTruthy()
    expect(screen.getByText('Add a Cargo Dispatch Note')).toBeTruthy()
  })

  it('folds the list away and back from its header', () => {
    renderServices()
    const toggle = screen.getByRole('button', { expanded: true })

    fireEvent.click(toggle)
    expect(toggle.getAttribute('aria-expanded')).toBe('false')
    expect(screen.queryByRole('list')).toBeNull()

    fireEvent.click(toggle)
    expect(toggle.getAttribute('aria-expanded')).toBe('true')
    expect(screen.getAllByRole('listitem')).toHaveLength(2)
  })

  it('shows nothing when no service can be started', () => {
    renderServices([])

    expect(screen.queryByRole('button')).toBeNull()
  })
})
