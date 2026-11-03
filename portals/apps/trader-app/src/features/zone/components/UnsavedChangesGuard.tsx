import { useCallback, useEffect, useState, type ReactNode } from 'react'
import { useBlocker } from 'react-router-dom'
import { ConfirmDialog } from '@portals/ui'
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
      <ConfirmDialog
        open={blocker.state === 'blocked'}
        onOpenChange={(open) => !open && blocker.reset?.()}
        title={t('tasks.unsaved.title')}
        description={t('tasks.unsaved.description')}
        confirmLabel={t('tasks.unsaved.leave')}
        cancelLabel={t('tasks.unsaved.stay')}
        onConfirm={() => blocker.proceed?.()}
        color="red"
      />
    </UnsavedChangesContext.Provider>
  )
}
