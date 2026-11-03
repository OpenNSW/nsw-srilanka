import { useState } from 'react'
import { Theme } from '@radix-ui/themes'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'
import { ConfirmDialog } from './ConfirmDialog'

function Harness({ busy = false, onConfirm = () => {} }: { busy?: boolean; onConfirm?: () => void }) {
  const [open, setOpen] = useState(true)
  return (
    <Theme>
      <ConfirmDialog
        open={open}
        onOpenChange={setOpen}
        title="Start the export flow?"
        description="A started flow cannot be withdrawn."
        confirmLabel="Start"
        cancelLabel="Cancel"
        onConfirm={onConfirm}
        busy={busy}
      >
        <p>Export to Rotterdam</p>
      </ConfirmDialog>
    </Theme>
  )
}

describe('ConfirmDialog', () => {
  it('shows its title, description and details, and confirms', async () => {
    const user = userEvent.setup()
    const onConfirm = vi.fn()
    render(<Harness onConfirm={onConfirm} />)

    const dialog = screen.getByRole('alertdialog', { name: 'Start the export flow?' })
    expect(dialog.textContent).toContain('A started flow cannot be withdrawn.')
    expect(dialog.textContent).toContain('Export to Rotterdam')

    await user.click(screen.getByRole('button', { name: 'Start' }))
    expect(onConfirm).toHaveBeenCalledOnce()
  })

  it('closes on Cancel', async () => {
    const user = userEvent.setup()
    render(<Harness />)

    await user.click(screen.getByRole('button', { name: 'Cancel' }))
    expect(screen.queryByRole('alertdialog')).toBeNull()
  })

  it('closes on Escape', async () => {
    const user = userEvent.setup()
    render(<Harness />)

    await user.keyboard('{Escape}')
    expect(screen.queryByRole('alertdialog')).toBeNull()
  })

  it('cannot be closed while busy', async () => {
    const user = userEvent.setup()
    render(<Harness busy />)

    expect(screen.getByRole<HTMLButtonElement>('button', { name: 'Cancel' }).disabled).toBe(true)
    await user.keyboard('{Escape}')
    expect(screen.getByRole('alertdialog')).toBeTruthy()
  })
})
