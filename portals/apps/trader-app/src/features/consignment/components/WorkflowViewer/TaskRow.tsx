import { Link } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { Badge, Button } from '@radix-ui/themes'
import { ChevronRightIcon, InfoCircledIcon } from '@radix-ui/react-icons'
import type { WorkflowNode, WorkflowNodeState } from '@/features/consignment/types'
import { WORKFLOW_STATUS_I18N_KEYS } from '@/features/consignment/workflowStatus'
import { formatDateTime } from '@/features/consignment/utils'

const STATUS_COLORS: Record<WorkflowNodeState, 'green' | 'blue' | 'orange' | 'gray' | 'red'> = {
  COMPLETED: 'green',
  READY: 'blue',
  IN_PROGRESS: 'blue',
  QUEUED_EXTERNALLY: 'orange',
  LOCKED: 'gray',
  FAILED: 'red',
}

// One task of a consignment. A task that needs action is a highlighted card
// with the page's main button; the others are plain rows that can be opened.
export function TaskRow({
  step,
  consignmentId,
  needsAction = false,
}: {
  step: WorkflowNode
  consignmentId: string
  needsAction?: boolean
}) {
  const { t } = useTranslation()
  const name = step.workflowNodeTemplate.name || t('workflow.untitledTask')
  const href = `/consignments/${consignmentId}/tasks/${step.id}`
  const status = (
    <Badge color={STATUS_COLORS[step.state] ?? 'gray'} variant="soft" size="1">
      {t(`workflow.status.${WORKFLOW_STATUS_I18N_KEYS[step.state] ?? 'inProgress'}`)}
    </Badge>
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

  if (needsAction) {
    return (
      <li className="flex flex-wrap items-center justify-between gap-3 rounded-xl border border-info/40 bg-info-subtle/40 px-4 py-3">
        <div className="min-w-0">
          <div className="flex flex-wrap items-center gap-2">
            <span className="font-semibold text-foreground">{name}</span>
            {status}
          </div>
          {step.workflowNodeTemplate.description && (
            <p className="mt-1 text-sm text-foreground-muted">{step.workflowNodeTemplate.description}</p>
          )}
          {note}
        </div>
        {step.state !== 'LOCKED' && (
          <Button asChild size="2">
            <Link to={href} aria-label={t('workflow.continueTask', { name })}>
              {t('workflow.continue')}
              <ChevronRightIcon aria-hidden />
            </Link>
          </Button>
        )}
      </li>
    )
  }

  return (
    <li className="flex flex-wrap items-center justify-between gap-3 border-b border-border px-1 py-3 last:border-b-0">
      <div className="min-w-0">
        <div className="flex flex-wrap items-center gap-2">
          <span className="font-medium text-foreground">{name}</span>
          {status}
        </div>
        {when && <p className="mt-0.5 text-xs text-foreground-muted">{when}</p>}
        {note}
      </div>
      <Link
        to={href}
        aria-label={t('workflow.viewTask', { name })}
        className="inline-flex items-center gap-1 text-sm font-medium text-primary hover:underline"
      >
        {t('workflow.view')}
        <ChevronRightIcon aria-hidden />
      </Link>
    </li>
  )
}
