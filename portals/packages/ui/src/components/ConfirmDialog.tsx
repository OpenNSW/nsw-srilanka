import type { ReactNode } from 'react'
import { AlertDialog, Button, Flex, type ButtonProps } from '@radix-ui/themes'

interface ConfirmDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  title: string
  description: ReactNode
  confirmLabel: string
  cancelLabel: string
  // The dialog stays open: close it here, or once the action it starts ends.
  onConfirm: () => void
  // While the action runs: the confirm button spins, and Cancel, Escape and
  // a click outside cannot close the dialog.
  busy?: boolean
  confirmDisabled?: boolean
  // The confirm button's color, such as red for an action that destroys.
  color?: ButtonProps['color']
  // Shown between the description and the buttons: details, or a field to fill.
  children?: ReactNode
}

// Asks before an action that can't be undone.
export function ConfirmDialog({
  open,
  onOpenChange,
  title,
  description,
  confirmLabel,
  cancelLabel,
  onConfirm,
  busy = false,
  confirmDisabled = false,
  color,
  children,
}: ConfirmDialogProps) {
  return (
    <AlertDialog.Root open={open} onOpenChange={(next) => !busy && onOpenChange(next)}>
      <AlertDialog.Content maxWidth="480px">
        <AlertDialog.Title>{title}</AlertDialog.Title>
        <AlertDialog.Description size="2">{description}</AlertDialog.Description>
        {children && <div className="mt-4">{children}</div>}
        <Flex gap="3" mt="4" justify="end">
          <AlertDialog.Cancel>
            <Button variant="soft" color="gray" disabled={busy}>
              {cancelLabel}
            </Button>
          </AlertDialog.Cancel>
          <Button color={color} loading={busy} disabled={confirmDisabled} onClick={onConfirm}>
            {confirmLabel}
          </Button>
        </Flex>
      </AlertDialog.Content>
    </AlertDialog.Root>
  )
}
