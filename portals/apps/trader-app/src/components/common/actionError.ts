import { createContext, useContext } from 'react'

// The action half of the shared error pattern. Page loads use LoadError
// (a message and Try Again). A failed action keeps the screen and raises
// this toast, because the control the trader just used is the way to try again.
export interface ActionErrorApi {
  // Returns the id of the toast it showed. Pass that id to clearActionError
  // to dismiss it only if a later failure has not replaced it.
  showActionError: (message: string) => number
  clearActionError: (id?: number) => void
}

export const ActionErrorContext = createContext<ActionErrorApi | null>(null)

export function useActionError(): ActionErrorApi {
  const value = useContext(ActionErrorContext)
  if (!value) {
    throw new Error('useActionError must be used within ActionErrorProvider')
  }
  return value
}
