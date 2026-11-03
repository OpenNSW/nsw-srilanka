import { Link } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { Button } from '@radix-ui/themes'
import { StatusBadge } from '@portals/ui'
import { ChevronRightIcon, InfoCircledIcon } from '@radix-ui/react-icons'
import type { WorkflowNode } from '@/features/consignment/types'
import { WORKFLOW_STATUS_I18N_KEYS, WORKFLOW_STATUS_TONES } from '@/features/consignment/workflowStatus'
import { formatDateTime } from '@/features/consignment/utils'
import { ROUTES } from '@/constants'

// One task of a consignment, as a card that opens it.
export function TaskRow({
  step,
  consignmentId,
  needsAction = false,
  canOpen,
  canAct,
}: {
  step: WorkflowNode
  consignmentId: string
  needsAction?: boolean
  // Whether the user may open a task. Without it the row has no link.
  canOpen: boolean
  // Whether the user may act on a task. Without it a task that needs action
  // offers View, not Continue.
  canAct: boolean
}) {
  const { t } = useTranslation()
  const name = step.workflowNodeTemplate.name || t('workflow.untitledTask')
  const href = ROUTES.TASK_FORM(consignmentId, step.id)
  const status = (
    <StatusBadge tone={WORKFLOW_STATUS_TONES[step.state] ?? 'neutral'}>
      {t(`workflow.status.${WORKFLOW_STATUS_I18N_KEYS[step.state] ?? 'inProgress'}`)}
    </StatusBadge>
  )
  const when =
    step.state === 'COMPLETED'
      ? t('workflow.completedAt', { date: formatDateTime(step.updatedAt) })
      : step.state === 'QUEUED_EXTERNALLY'
        ? t('workflow.since', { date: formatDateTime(step.updatedAt) })
        : undefined
  const note = step.extendedState && (
    <p className="mt-1 flex items-center gap-1 text-xs font-medium text-warning-strong">
      <InfoCircledIcon aria-hidden />
      {step.extendedState}
    </p>
  )

  // Every task is the same card; one that needs action is tinted and holds
  // the page's main button.
  return (
    <li
      className={`flex flex-wrap items-center justify-between gap-3 rounded-xl border px-4 py-3 ${
        needsAction ? 'border-info/40 bg-info-subtle/40' : 'border-border bg-app-surface'
      }`}
    >
      <div className="min-w-0">
        <div className="flex flex-wrap items-center gap-2">
          <span className="font-semibold text-foreground">{name}</span>
          {status}
        </div>
        {step.workflowNodeTemplate.description && (
          <p className="mt-1 text-sm text-foreground-muted">{step.workflowNodeTemplate.description}</p>
        )}
        {when && <p className="mt-0.5 text-xs text-foreground-muted">{when}</p>}
        {note}
      </div>
      {!canOpen ? null : needsAction && canAct ? (
        step.state !== 'LOCKED' && (
          <Button asChild size="2">
            <Link to={href} aria-label={t('workflow.continueTask', { name })}>
              {t('workflow.continue')}
              <ChevronRightIcon aria-hidden />
            </Link>
          </Button>
        )
      ) : (
        <Link
          to={href}
          aria-label={t('workflow.viewTask', { name })}
          className="inline-flex items-center gap-1 text-sm font-medium text-primary hover:underline"
        >
          {t('workflow.view')}
          <ChevronRightIcon aria-hidden />
        </Link>
      )}
    </li>
  )
}
