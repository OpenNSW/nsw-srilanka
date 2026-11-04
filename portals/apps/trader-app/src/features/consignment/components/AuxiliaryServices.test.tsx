import { Theme } from '@radix-ui/themes'
import { fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
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

function renderServices(list = flows, onStart = vi.fn().mockResolvedValue(undefined)) {
  render(
    <Theme>
      <AuxiliaryServices flows={list} onStart={onStart} />
    </Theme>,
  )
  return onStart
}

// Opens the list and presses Start on the service at index, which asks to confirm.
function pressStart(index: number) {
  fireEvent.click(screen.getByRole('button', { expanded: false }))
  fireEvent.click(screen.getAllByRole('button', { name: 'consignments.detail.flows.startService' })[index])
  return screen.getByRole('alertdialog')
}

describe('AuxiliaryServices', () => {
  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('starts closed', () => {
    renderServices()

    const toggle = screen.getByRole('button', { expanded: false })
    expect(toggle.getAttribute('aria-controls')).toBe('auxiliary-services-list')
    expect(screen.queryByRole('list')).toBeNull()
  })

  it('opens to a card per service from its header, and folds them away again', () => {
    renderServices()
    const toggle = screen.getByRole('button', { expanded: false })

    fireEvent.click(toggle)
    expect(toggle.getAttribute('aria-expanded')).toBe('true')
    expect(screen.getAllByRole('listitem')).toHaveLength(2)
    expect(screen.getByText('SLPA sundry payment')).toBeTruthy()

    fireEvent.click(toggle)
    expect(toggle.getAttribute('aria-expanded')).toBe('false')
    expect(screen.queryByRole('list')).toBeNull()
  })

  it('shows nothing when no service can be started', () => {
    renderServices([])

    expect(screen.queryByRole('button')).toBeNull()
  })

  it('asks to confirm before starting, and starts nothing on cancel', () => {
    const onStart = renderServices()

    const dialog = pressStart(1)
    expect(within(dialog).getByText('consignments.detail.flows.confirmTitle')).toBeTruthy()

    fireEvent.click(within(dialog).getByRole('button', { name: 'consignments.detail.flows.cancel' }))
    expect(screen.queryByRole('alertdialog')).toBeNull()
    expect(onStart).not.toHaveBeenCalled()
  })

  it('starts the chosen service on confirm, and closes the dialog', async () => {
    const onStart = renderServices()

    const dialog = pressStart(1)
    fireEvent.click(within(dialog).getByRole('button', { name: 'consignments.detail.flows.start' }))

    expect(onStart).toHaveBeenCalledExactlyOnceWith('customs-additional-cdn')
    await waitFor(() => expect(screen.queryByRole('alertdialog')).toBeNull())
    expect(screen.queryByText('consignments.detail.flows.failed')).toBeNull()
  })

  it('says a service could not start, and closes the dialog', async () => {
    vi.spyOn(console, 'error').mockImplementation(() => {})
    renderServices(flows, vi.fn().mockRejectedValue(new Error('conflict')))

    const dialog = pressStart(0)
    fireEvent.click(within(dialog).getByRole('button', { name: 'consignments.detail.flows.start' }))

    expect(await screen.findByText('consignments.detail.flows.failed')).toBeTruthy()
    expect(screen.queryByRole('alertdialog')).toBeNull()
  })
})
