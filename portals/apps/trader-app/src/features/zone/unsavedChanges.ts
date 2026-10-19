import { createContext, useContext, useEffect, useId } from 'react'

// ReportUnsaved records whether the form with this id holds input that has not
// been saved (as a draft or a submission) yet.
export type ReportUnsaved = (id: string, unsaved: boolean) => void

// Provided by UnsavedChangesGuard. Absent elsewhere (e.g. the dev zone
// preview), where a form's unsaved input guards nothing.
export const UnsavedChangesContext = createContext<ReportUnsaved | null>(null)

// useReportUnsaved tells the enclosing UnsavedChangesGuard whether this form
// has unsaved input. Unmounting clears it, so a form that remounts against
// fresh data (a new state, a refetch after a save) starts out clean.
export function useReportUnsaved(unsaved: boolean) {
  const report = useContext(UnsavedChangesContext)
  const id = useId()
  useEffect(() => {
    if (!report) return
    report(id, unsaved)
    return () => report(id, false)
  }, [report, id, unsaved])
}
