import { useMemo } from 'react'
import { Tabs } from '@radix-ui/themes'
import { useTranslation } from 'react-i18next'
import type { ConsignmentState, WorkflowNode } from '@/features/consignment/types'
import { isTraderVisibleNodeType } from '@/features/consignment/workflowNodes'
import { TaskRow } from './TaskRow'

const newestFirst = (a: WorkflowNode, b: WorkflowNode) => b.updatedAt.localeCompare(a.updatedAt)

// A consignment's tasks in two tabs. Active holds everything still in flight,
// what needs action first and then what is with someone else, so nothing in
// progress is a tab away; Done holds the finished ones. Tasks appear as the
// workflows reach them, so nothing here counts steps still to come.
export function ConsignmentTasks({
  steps,
  consignmentId,
  consignmentState,
}: {
  steps: WorkflowNode[]
  consignmentId: string
  consignmentState?: ConsignmentState
}) {
  const { t } = useTranslation()

  const groups = useMemo(() => {
    const visible = steps.filter((step) => isTraderVisibleNodeType(step.workflowNodeTemplate.type))
    const done = visible.filter((s) => s.state === 'COMPLETED' || s.state === 'FAILED').sort(newestFirst)
    const waiting = visible.filter((s) => s.state === 'QUEUED_EXTERNALLY').sort(newestFirst)
    const placed = new Set([...done, ...waiting].map((s) => s.id))
    return { needsAction: visible.filter((s) => !placed.has(s.id)), waiting, done }
  }, [steps])

  const finished = consignmentState === 'FINISHED'
  const activeCount = groups.needsAction.length + groups.waiting.length
  const footer = finished
    ? t('workflow.consignmentComplete')
    : groups.needsAction.length === 0
      ? `${t('workflow.nothingNeeded')} ${t('workflow.newStepsAppear')}`
      : t('workflow.newStepsAppear')

  return (
    <Tabs.Root defaultValue={finished ? 'done' : 'active'}>
      <Tabs.List>
        <Tabs.Trigger value="active">
          {t('workflow.tabs.active')} ({activeCount})
        </Tabs.Trigger>
        <Tabs.Trigger value="done">
          {t('workflow.tabs.done')} ({groups.done.length})
        </Tabs.Trigger>
      </Tabs.List>

      <Tabs.Content value="active" className="pt-5">
        {groups.needsAction.length > 0 && (
          <section className="mb-6">
            <h2 className="mb-3 text-xs font-semibold tracking-wide text-foreground-muted uppercase">
              {t('workflow.actionRequired')}
            </h2>
            <ul className="space-y-3">
              {groups.needsAction.map((step) => (
                <TaskRow key={step.id} step={step} consignmentId={consignmentId} needsAction />
              ))}
            </ul>
          </section>
        )}
        {groups.waiting.length > 0 && (
          <section className="mb-6">
            <h2 className="mb-1 text-xs font-semibold tracking-wide text-foreground-muted uppercase">
              {t('workflow.waitingOnOthers')}
            </h2>
            <ul>
              {groups.waiting.map((step) => (
                <TaskRow key={step.id} step={step} consignmentId={consignmentId} />
              ))}
            </ul>
          </section>
        )}
        <p className="text-sm text-foreground-muted">{footer}</p>
      </Tabs.Content>

      <Tabs.Content value="done" className="pt-5">
        {finished && <p className="mb-3 text-sm text-foreground-muted">{t('workflow.consignmentComplete')}</p>}
        {groups.done.length > 0 ? (
          <ul>
            {groups.done.map((step) => (
              <TaskRow key={step.id} step={step} consignmentId={consignmentId} />
            ))}
          </ul>
        ) : (
          <p className="text-sm text-foreground-muted">{t('workflow.noneDone')}</p>
        )}
      </Tabs.Content>
    </Tabs.Root>
  )
}
