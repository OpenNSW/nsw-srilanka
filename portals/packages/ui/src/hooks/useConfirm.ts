import { useCallback, useMemo, useState } from 'react'

export interface UseConfirmResult<T> {
  // What the dialog asks about, null until the first ask. It stays set after the dialog closes,
  // so the dialog's text holds through its closing animation.
  subject: T | null
  open: boolean
  // Opens the dialog about subject.
  ask: (subject: T) => void
  close: () => void
  // Spread onto ConfirmDialog. Busy stays with the caller, which usually needs it outside the
  // dialog too.
  dialogProps: { open: boolean; onOpenChange: (open: boolean) => void }
}

// The state behind a ConfirmDialog: whether it is open, and what it asks about. With no type
// argument there is no subject, and ask() takes none.
export function useConfirm<T = void>(): UseConfirmResult<T> {
  const [subject, setSubject] = useState<T | null>(null)
  const [open, setOpen] = useState(false)

  const ask = useCallback((next: T) => {
    setSubject(() => next)
    setOpen(true)
  }, [])
  const close = useCallback(() => setOpen(false), [])
  const dialogProps = useMemo(() => ({ open, onOpenChange: setOpen }), [open])

  return { subject, open, ask, close, dialogProps }
}
