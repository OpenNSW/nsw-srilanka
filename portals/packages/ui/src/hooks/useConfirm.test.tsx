import { Theme } from '@radix-ui/themes'
import { act, render, renderHook, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it } from 'vitest'
import { ConfirmDialog } from '../components/ConfirmDialog'
import { useConfirm } from './useConfirm'

describe('useConfirm', () => {
  it('starts closed, with nothing to confirm', () => {
    const { result } = renderHook(() => useConfirm<string>())

    expect(result.current.open).toBe(false)
    expect(result.current.subject).toBeNull()
    expect(result.current.dialogProps.open).toBe(false)
  })

  it('opens about the subject it is asked about', () => {
    const { result } = renderHook(() => useConfirm<string>())

    act(() => result.current.ask('export flow'))

    expect(result.current.open).toBe(true)
    expect(result.current.dialogProps.open).toBe(true)
    expect(result.current.subject).toBe('export flow')
  })

  it('keeps the subject after it closes', () => {
    const { result } = renderHook(() => useConfirm<string>())
    act(() => result.current.ask('export flow'))

    act(() => result.current.close())

    expect(result.current.open).toBe(false)
    expect(result.current.subject).toBe('export flow')
  })

  it('follows the dialog closing itself', () => {
    const { result } = renderHook(() => useConfirm<string>())
    act(() => result.current.ask('export flow'))

    act(() => result.current.dialogProps.onOpenChange(false))

    expect(result.current.open).toBe(false)
    expect(result.current.subject).toBe('export flow')
  })

  it('asks about the latest subject', () => {
    const { result } = renderHook(() => useConfirm<string>())
    act(() => result.current.ask('export flow'))
    act(() => result.current.close())

    act(() => result.current.ask('sundry payment'))

    expect(result.current.open).toBe(true)
    expect(result.current.subject).toBe('sundry payment')
  })

  it('holds a function as the subject rather than calling it', () => {
    const { result } = renderHook(() => useConfirm<() => string>())
    const subject = () => 'called'

    act(() => result.current.ask(subject))

    expect(result.current.subject).toBe(subject)
  })

  it('needs no subject for a plain yes/no question', () => {
    const { result } = renderHook(() => useConfirm())

    act(() => result.current.ask())

    expect(result.current.open).toBe(true)
  })

  it('keeps the same functions across renders', () => {
    const { result, rerender } = renderHook(() => useConfirm<string>())
    const { ask, close } = result.current

    rerender()

    expect(result.current.ask).toBe(ask)
    expect(result.current.close).toBe(close)
  })
})

function FlowStarter() {
  const confirm = useConfirm<{ label: string }>()
  return (
    <Theme>
      <button onClick={() => confirm.ask({ label: 'Sundry payment' })}>Start</button>
      <ConfirmDialog
        {...confirm.dialogProps}
        title={`Start ${confirm.subject?.label ?? ''}?`}
        description="A started flow cannot be withdrawn."
        confirmLabel="Start"
        cancelLabel="Cancel"
        onConfirm={confirm.close}
      />
    </Theme>
  )
}

describe('useConfirm with ConfirmDialog', () => {
  it('opens the dialog about the subject and closes it on Cancel', async () => {
    const user = userEvent.setup()
    render(<FlowStarter />)

    await user.click(screen.getByRole('button', { name: 'Start' }))
    expect(screen.getByRole('alertdialog', { name: 'Start Sundry payment?' })).toBeTruthy()

    await user.click(screen.getByRole('button', { name: 'Cancel' }))
    expect(screen.queryByRole('alertdialog')).toBeNull()
  })

  it('closes when the caller confirms and closes it', async () => {
    const user = userEvent.setup()
    render(<FlowStarter />)
    await user.click(screen.getByRole('button', { name: 'Start' }))

    await user.click(within(screen.getByRole('alertdialog')).getByRole('button', { name: 'Start' }))

    expect(screen.queryByRole('alertdialog')).toBeNull()
  })
})
