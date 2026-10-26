import { useCallback, useEffect, useState, type ReactNode } from 'react'
import { useBlocker } from 'react-router-dom'
import { AlertDialog, Button } from '@radix-ui/themes'
import { useTranslation } from 'react-i18next'
import { UnsavedChangesContext, type ReportUnsaved } from '@/features/zone/unsavedChanges'

// UnsavedChangesGuard asks before the user leaves while a form inside it holds
// unsaved input: an in-app navigation (Back to Tasks, the nav menu, the
// browser's back button) opens a dialog, and a reload or closing the tab gets
// the browser's own prompt. Forms report their state with useReportUnsaved.
// One guard per screen: a router supports only one blocker at a time.
export function UnsavedChangesGuard({ children }: { children: ReactNode }) {
  const { t } = useTranslation()
  const [unsavedIds, setUnsavedIds] = useState<ReadonlySet<string>>(() => new Set())
  const report = useCallback<ReportUnsaved>((id, unsaved) => {
    setUnsavedIds((prev) => {
      if (prev.has(id) === unsaved) return prev
      const next = new Set(prev)
      if (unsaved) next.add(id)
      else next.delete(id)
      return next
    })
  }, [])
  const hasUnsaved = unsavedIds.size > 0

  const blocker = useBlocker(
    ({ currentLocation, nextLocation }) => hasUnsaved && currentLocation.pathname !== nextLocation.pathname,
  )

  useEffect(() => {
    if (!hasUnsaved) return
    const onBeforeUnload = (event: BeforeUnloadEvent) => {
      event.preventDefault()
      // Still required by some browsers to show the prompt.
      event.returnValue = ''
    }
    window.addEventListener('beforeunload', onBeforeUnload)
    return () => window.removeEventListener('beforeunload', onBeforeUnload)
  }, [hasUnsaved])

  return (
    <UnsavedChangesContext.Provider value={report}>
      {children}
      <AlertDialog.Root open={blocker.state === 'blocked'} onOpenChange={(open) => !open && blocker.reset?.()}>
        <AlertDialog.Content maxWidth="480px">
          <AlertDialog.Title className="mb-3">{t('tasks.unsaved.title')}</AlertDialog.Title>
          <AlertDialog.Description size="2" className="mb-4">
            {t('tasks.unsaved.description')}
          </AlertDialog.Description>
          <div className="flex justify-end gap-3">
            <AlertDialog.Cancel>
              <Button variant="soft" color="gray">
                {t('tasks.unsaved.stay')}
              </Button>
            </AlertDialog.Cancel>
            <AlertDialog.Action>
              <Button color="red" onClick={() => blocker.proceed?.()}>
                {t('tasks.unsaved.leave')}
              </Button>
            </AlertDialog.Action>
          </div>
        </AlertDialog.Content>
      </AlertDialog.Root>
    </UnsavedChangesContext.Provider>
  )
}
