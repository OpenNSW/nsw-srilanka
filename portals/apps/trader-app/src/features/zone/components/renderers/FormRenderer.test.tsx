import { act, fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { Theme } from '@radix-ui/themes'
import { afterEach, describe, expect, it, vi } from 'vitest'
import '@/i18n'
import { FormRenderer } from '@/features/zone/components/renderers/FormRenderer'
import type { FormPayload, Handle, ZoneAction } from '@/features/zone/types'

const schema = {
  type: 'object',
  properties: { name: { type: 'string', title: 'Name' } },
  required: ['name'],
} as FormPayload['schema']

const uiSchema = {
  type: 'VerticalLayout',
  elements: [{ type: 'Control', scope: '#/properties/name' }],
} as FormPayload['uiSchema']

const draft: Handle = { command: 'save', label: 'Save as Draft', element: 'draft_action' }
const submit: Handle = { command: 'submit', label: 'Submit', element: 'primary_action' }
const sendBack: Handle = { command: 'send_back', label: 'Send Back for Amendment', element: 'secondary_action' }
const cancelCdn: Handle = { command: 'cancel_cdn', label: 'Cancel CDN', element: 'danger_action' }

function renderForm(
  data: Record<string, unknown>,
  onAction: ZoneAction,
  handles = [draft, submit, sendBack, cancelCdn],
) {
  render(
    <Theme>
      <FormRenderer payload={{ schema, uiSchema, data }} handles={handles} onAction={onAction} />
    </Theme>,
  )
}

// The form's own button for a handle. While a dialog is open the rest of the
// page is hidden from the accessibility tree, so this looks only when none is.
function formButton(name: string) {
  return screen.getByRole('button', { name })
}

// Types into the form's one field, under fake timers. JsonForms reports the
// form's data a moment after mount and after each edit: the first report is
// what counts as saved, so the edit waits for it, and the autosave timer
// starts from the edit's own report.
async function typeName(value: string) {
  await act(async () => vi.advanceTimersByTimeAsync(100))
  fireEvent.change(screen.getByRole('textbox'), { target: { value } })
  await act(async () => vi.advanceTimersByTimeAsync(100))
}

describe('FormRenderer', () => {
  afterEach(() => {
    vi.useRealTimers()
  })

  it('asks before a primary action, and sends it once confirmed', async () => {
    const onAction = vi.fn<ZoneAction>().mockResolvedValue(undefined)
    renderForm({ name: 'Tea' }, onAction)

    fireEvent.click(formButton('Submit'))

    const dialog = screen.getByRole('alertdialog', { name: 'Submit?' })
    expect(within(dialog).getByText(/can't be changed or undone/)).toBeTruthy()
    expect(onAction).not.toHaveBeenCalled()

    fireEvent.click(within(dialog).getByRole('button', { name: 'Submit' }))

    expect(onAction).toHaveBeenCalledWith('submit', { name: 'Tea' }, { draft: false })
    await waitFor(() => expect(screen.queryByRole('alertdialog')).toBeNull())
  })

  it('sends nothing when the user goes back', async () => {
    const onAction = vi.fn<ZoneAction>().mockResolvedValue(undefined)
    renderForm({ name: 'Tea' }, onAction)

    fireEvent.click(formButton('Submit'))
    fireEvent.click(within(screen.getByRole('alertdialog')).getByRole('button', { name: 'Go back' }))

    await waitFor(() => expect(screen.queryByRole('alertdialog')).toBeNull())
    expect(onAction).not.toHaveBeenCalled()
  })

  it('shows the missing fields, not the dialog, when a primary action needs a complete form', () => {
    const onAction = vi.fn<ZoneAction>().mockResolvedValue(undefined)
    renderForm({}, onAction)

    fireEvent.click(formButton('Submit'))

    expect(screen.getByText('Please fill in all required fields.')).toBeTruthy()
    expect(screen.queryByRole('alertdialog')).toBeNull()
    expect(onAction).not.toHaveBeenCalled()
  })

  it('asks before a secondary action, which sends an incomplete form', () => {
    const onAction = vi.fn<ZoneAction>().mockResolvedValue(undefined)
    renderForm({}, onAction)

    fireEvent.click(formButton('Send Back for Amendment'))
    const dialog = screen.getByRole('alertdialog', { name: 'Send Back for Amendment?' })
    fireEvent.click(within(dialog).getByRole('button', { name: 'Send Back for Amendment' }))

    expect(onAction).toHaveBeenCalledWith('send_back', {}, { draft: false })
  })

  it('saves a draft at once, without asking', () => {
    const onAction = vi.fn<ZoneAction>().mockResolvedValue(undefined)
    renderForm({}, onAction)

    fireEvent.click(formButton('Save as Draft'))

    expect(screen.queryByRole('alertdialog')).toBeNull()
    expect(onAction).toHaveBeenCalledWith('save', {}, { draft: true })
  })

  it('colours the confirm button of a danger action red', () => {
    const onAction = vi.fn<ZoneAction>().mockResolvedValue(undefined)
    renderForm({ name: 'Tea' }, onAction)

    fireEvent.click(formButton('Cancel CDN'))

    const dialog = screen.getByRole('alertdialog', { name: 'Cancel CDN?' })
    expect(within(dialog).getByRole('button', { name: 'Cancel CDN' }).getAttribute('data-accent-color')).toBe('red')
  })

  it('keeps the dialog open and busy while the action is in flight', async () => {
    let finish: () => void = () => {}
    const onAction = vi.fn<ZoneAction>(() => new Promise<void>((resolve) => (finish = resolve)))
    renderForm({ name: 'Tea' }, onAction)

    fireEvent.click(formButton('Submit'))
    const dialog = screen.getByRole('alertdialog')
    fireEvent.click(within(dialog).getByRole('button', { name: 'Submit' }))

    expect(within(dialog).getByRole('button', { name: 'Go back' }).hasAttribute('disabled')).toBe(true)
    fireEvent.keyDown(dialog, { key: 'Escape' })
    expect(screen.getByRole('alertdialog')).toBe(dialog)
    expect(onAction).toHaveBeenCalledTimes(1)

    act(() => finish())
    await waitFor(() => expect(screen.queryByRole('alertdialog')).toBeNull())
  })

  it('closes the dialog and enables the buttons again when the action fails', async () => {
    const onAction = vi.fn<ZoneAction>().mockRejectedValue(new Error('rejected'))
    renderForm({ name: 'Tea' }, onAction)

    fireEvent.click(formButton('Submit'))
    fireEvent.click(within(screen.getByRole('alertdialog')).getByRole('button', { name: 'Submit' }))

    await waitFor(() => expect(screen.queryByRole('alertdialog')).toBeNull())
    expect(formButton('Submit').hasAttribute('disabled')).toBe(false)
  })

  it('autosaves a draft without asking', async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true })
    const onAction = vi.fn<ZoneAction>().mockResolvedValue(undefined)
    renderForm({ name: 'Tea' }, onAction)
    await typeName('Coffee')

    await act(async () => vi.advanceTimersByTimeAsync(30_000))

    expect(onAction).toHaveBeenCalledWith('save', { name: 'Coffee' }, { draft: true })
    expect(screen.queryByRole('alertdialog')).toBeNull()
  })

  it('holds the autosave while the dialog is open', async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true })
    const onAction = vi.fn<ZoneAction>().mockResolvedValue(undefined)
    renderForm({ name: 'Tea' }, onAction)
    await typeName('Coffee')

    fireEvent.click(formButton('Submit'))
    await act(async () => vi.advanceTimersByTimeAsync(30_000))

    expect(onAction).not.toHaveBeenCalled()
    expect(screen.getByRole('alertdialog', { name: 'Submit?' })).toBeTruthy()
  })
})
