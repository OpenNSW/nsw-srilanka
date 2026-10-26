import { useState, useEffect, useSyncExternalStore, type ReactNode } from 'react'
import { IconButton, Text } from '@radix-ui/themes'
import { Cross2Icon } from '@radix-ui/react-icons'
import { useTranslation } from 'react-i18next'
import { useLocation } from 'react-router-dom'
import { ActionErrorContext, createActionErrorStore, useActionError } from '@/components/common/actionError'

// useState calls a function initializer once, which is how the store is created.
export function ActionErrorProvider({ children }: { children: ReactNode }) {
  const [store] = useState(createActionErrorStore)

  return (
    <ActionErrorContext.Provider value={store.api}>
      {children}
      <ActionErrorToast store={store} />
    </ActionErrorContext.Provider>
  )
}

function ActionErrorToast({ store }: { store: ReturnType<typeof createActionErrorStore> }) {
  const { t } = useTranslation()
  const toast = useSyncExternalStore(store.subscribe, store.getSnapshot, store.getSnapshot)
  const dismiss = () => store.api.clearActionError()

  useEffect(() => {
    if (!toast) return
    const onKeyDown = (event: KeyboardEvent) => {
      // A dialog that already consumed Escape must keep the toast.
      if (event.key !== 'Escape' || event.defaultPrevented) return
      store.api.clearActionError()
    }
    window.addEventListener('keydown', onKeyDown)
    return () => window.removeEventListener('keydown', onKeyDown)
  }, [toast, store])

  if (!toast) return null

  return (
    <div className="pointer-events-none fixed inset-x-0 top-[calc(var(--app-top-offset)+0.75rem)] z-[60] flex justify-center px-4">
      <div
        key={toast.id}
        role="alert"
        className="pointer-events-auto flex max-w-md items-start gap-3 rounded-xl bg-error-subtle px-4 py-3 shadow-lg"
      >
        <Text size="2" weight="medium" className="min-w-0 flex-1 text-error-strong">
          {toast.message}
        </Text>
        <IconButton
          type="button"
          variant="ghost"
          color="red"
          size="1"
          className="shrink-0"
          aria-label={t('common.error.dismiss')}
          onClick={dismiss}
        >
          <Cross2Icon />
        </IconButton>
      </div>
    </div>
  )
}

// Rendered under the router. A failure belongs to the page the trader was on.
export function ClearActionErrorOnNavigate() {
  const { pathname } = useLocation()
  const { clearActionError } = useActionError()
  useEffect(() => {
    clearActionError()
  }, [pathname, clearActionError])
  return null
}
