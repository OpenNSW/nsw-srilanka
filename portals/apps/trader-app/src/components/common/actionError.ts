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

export interface ActionToast {
  id: number
  message: string
}

interface ActionErrorStore {
  api: ActionErrorApi
  subscribe: (listener: () => void) => () => void
  getSnapshot: () => ActionToast | null
}

export const ActionErrorContext = createContext<ActionErrorApi | null>(null)

export function useActionError(): ActionErrorApi {
  const value = useContext(ActionErrorContext)
  if (!value) {
    throw new Error('useActionError must be used within ActionErrorProvider')
  }
  return value
}

// One store per provider. Showing a toast notifies only the toast, not the
// screens, and a call works as soon as the provider has rendered.
export function createActionErrorStore(): ActionErrorStore {
  let toast: ActionToast | null = null
  let nextId = 0
  const listeners = new Set<() => void>()
  const emit = () => {
    listeners.forEach((listener) => listener())
  }

  return {
    subscribe: (listener) => {
      listeners.add(listener)
      return () => listeners.delete(listener)
    },
    getSnapshot: () => toast,
    api: {
      showActionError: (message) => {
        nextId += 1
        toast = { id: nextId, message }
        emit()
        return nextId
      },
      clearActionError: (id) => {
        if (!toast) return
        if (id !== undefined && toast.id !== id) return
        toast = null
        emit()
      },
    },
  }
}
