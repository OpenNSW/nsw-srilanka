import { Link } from 'react-router-dom'
import { ChevronRightIcon } from '@radix-ui/react-icons'
import { useTranslation } from 'react-i18next'

// Consignments › <consignment> › <task>: where the trader is, and the way back
// to each level.
export function TaskBreadcrumb({
  consignmentId,
  consignmentName,
  taskName,
}: {
  consignmentId?: string
  consignmentName?: string
  taskName?: string
}) {
  const { t } = useTranslation()
  const link = 'text-foreground-muted hover:text-foreground hover:underline'
  return (
    <nav aria-label={t('tasks.breadcrumb')} className="min-w-0">
      <ol className="flex flex-wrap items-center gap-1.5 text-sm">
        <li>
          <Link to="/consignments" className={link}>
            {t('nav.consignments')}
          </Link>
        </li>
        {consignmentId && (
          <li className="flex min-w-0 items-center gap-1.5">
            <ChevronRightIcon aria-hidden className="shrink-0 text-foreground-muted" />
            <Link to={`/consignments/${consignmentId}`} className={`${link} truncate max-w-64`}>
              {consignmentName || t('consignments.detail.title')}
            </Link>
          </li>
        )}
        {taskName && (
          <li className="flex min-w-0 items-center gap-1.5">
            <ChevronRightIcon aria-hidden className="shrink-0 text-foreground-muted" />
            <span aria-current="page" className="truncate font-medium text-foreground">
              {taskName}
            </span>
          </li>
        )}
      </ol>
    </nav>
  )
}
