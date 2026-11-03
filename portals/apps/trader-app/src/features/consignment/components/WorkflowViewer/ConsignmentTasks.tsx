import { useMemo } from 'react'
import type { ReactNode } from 'react'
import { Badge, Tabs } from '@radix-ui/themes'
import { ClockIcon, LightningBoltIcon } from '@radix-ui/react-icons'
import { useTranslation } from 'react-i18next'
import type { ConsignmentState, WorkflowNode } from '@/features/consignment/types'
import { isTraderVisibleNodeType } from '@/features/consignment/workflowNodes'
import { isAgencyMode } from '@/features/case/mode'
import { TaskRow } from './TaskRow'

function SectionHeading({ icon, title, count }: { icon: ReactNode; title: string; count: number }) {
  return (
    <h2 className="mb-3 flex items-center gap-2 text-sm font-semibold text-foreground">
      {icon}
      {title}
      <Badge color="gray" variant="soft" radius="full">
        {count}
      </Badge>
    </h2>
  )
}

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
    const done = visible
      .filter((s) => s.state === 'COMPLETED' || s.state === 'FAILED' || s.state === 'CANCELLED')
      .sort(newestFirst)
    const waiting = visible.filter((s) => s.state === 'QUEUED_EXTERNALLY' || s.state === 'ON_HOLD').sort(newestFirst)
    const placed = new Set([...done, ...waiting].map((s) => s.id))
    return { needsAction: visible.filter((s) => !placed.has(s.id)), waiting, done }
  }, [steps])

  const finished = consignmentState === 'FINISHED'
  const activeCount = groups.needsAction.length + groups.waiting.length
  // Where the actions would be, the tab says why there are none; under a list
  // of actions, that more can come is only a footnote.
  const status = finished
    ? t('workflow.consignmentComplete')
    : groups.needsAction.length === 0
      ? `${t('workflow.nothingNeeded')} ${t(isAgencyMode ? 'workflow.newStepsAppearCase' : 'workflow.newStepsAppear')}`
      : undefined

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
        {status && <p className="mb-6 text-sm text-foreground-muted">{status}</p>}
        {groups.needsAction.length > 0 && (
          <section>
            <SectionHeading
              icon={<LightningBoltIcon aria-hidden className="text-info-strong" />}
              title={t('workflow.actionRequired')}
              count={groups.needsAction.length}
            />
            <ul className="space-y-3">
              {groups.needsAction.map((step) => (
                <TaskRow key={step.id} step={step} consignmentId={consignmentId} needsAction />
              ))}
            </ul>
          </section>
        )}
        {groups.waiting.length > 0 && (
          <section className={groups.needsAction.length > 0 ? 'mt-8 border-t border-border pt-6' : undefined}>
            <SectionHeading
              icon={<ClockIcon aria-hidden className="text-warning-strong" />}
              title={t('workflow.waitingOnOthers')}
              count={groups.waiting.length}
            />
            <ul className="space-y-3">
              {groups.waiting.map((step) => (
                <TaskRow key={step.id} step={step} consignmentId={consignmentId} />
              ))}
            </ul>
          </section>
        )}
        {!status && (
          <p className="mt-6 text-sm text-foreground-muted">
            {t(isAgencyMode ? 'workflow.newStepsAppearCase' : 'workflow.newStepsAppear')}
          </p>
        )}
      </Tabs.Content>

      <Tabs.Content value="done" className="pt-5">
        {finished && <p className="mb-3 text-sm text-foreground-muted">{t('workflow.consignmentComplete')}</p>}
        {groups.done.length > 0 ? (
          <ul className="space-y-3">
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
