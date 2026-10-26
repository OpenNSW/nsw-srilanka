import {
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
  type Dispatch,
  type ReactNode,
  type SetStateAction,
} from 'react'
import { IconButton, Text } from '@radix-ui/themes'
import { Cross2Icon } from '@radix-ui/react-icons'
import { useTranslation } from 'react-i18next'
import { useLocation } from 'react-router-dom'
import { ActionErrorContext, useActionError } from '@/components/common/actionError'

interface ActionToast {
  id: number
  message: string
}

type SetToast = Dispatch<SetStateAction<ActionToast | null>>

// The toast state lives in ActionErrorToast, not here, so showing or
// dismissing one does not re-render the rest of the app.
export function ActionErrorProvider({ children }: { children: ReactNode }) {
  const setToastRef = useRef<SetToast>(() => {})
  const nextToastId = useRef(0)
  const api = useMemo(
    () => ({
      showActionError: (message: string) => {
        nextToastId.current += 1
        const id = nextToastId.current
        setToastRef.current({ id, message })
        return id
      },
      clearActionError: (id?: number) => {
        setToastRef.current((current) => {
          if (!current) return null
          if (id !== undefined && current.id !== id) return current
          return null
        })
      },
    }),
    [],
  )

  return (
    <ActionErrorContext.Provider value={api}>
      {children}
      <ActionErrorToast setToastRef={setToastRef} />
    </ActionErrorContext.Provider>
  )
}

function ActionErrorToast({ setToastRef }: { setToastRef: { current: SetToast } }) {
  const { t } = useTranslation()
  const [toast, setToastState] = useState<ActionToast | null>(null)

  useEffect(() => {
    setToastRef.current = setToastState
  }, [setToastRef])

  const dismiss = useCallback(() => {
    setToastState(null)
  }, [])

  useEffect(() => {
    if (!toast) return
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === 'Escape') dismiss()
    }
    window.addEventListener('keydown', onKeyDown)
    return () => window.removeEventListener('keydown', onKeyDown)
  }, [toast, dismiss])

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
