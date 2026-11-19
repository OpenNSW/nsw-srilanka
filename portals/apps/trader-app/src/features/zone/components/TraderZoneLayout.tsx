import type { Alert, AlertVariant, AuditEntry, ZoneAction, ZoneView } from '@/features/zone/types'
import { useTranslation } from 'react-i18next'
import type { TFunction } from 'i18next'
import { useMemo } from 'react'
import { SearchServiceProvider } from '@opennsw/jsonforms-renderers'
import { workflowStatusI18nKey, workflowStatusTone } from '@/features/consignment/workflowStatus'
import { humanizeStatus } from '@/utils/formatStatus'
import { Zone } from './Zone'
import { zoneWidthClass } from '@/features/zone/width'
import { Banner, CopyableId, StatusBadge, Surface } from '@portals/ui'
import { taskSearchServices } from '@/services/searchService'

type Props = {
  task: ZoneView
  onSubmitForm?: ZoneAction
  // Changes once an action's refetch has landed. Part of each zone's key, so a
  // step returning to the same form remounts it against the refreshed data
  // rather than leaving the values it mounted with on screen.
  formEpoch?: number
  // The task's name, as the deployment configured it (its render config's
  // title). Without one the title falls back to the task type.
  title?: string
}

export function TraderZoneLayout({ task, onSubmitForm, formEpoch = 0, title }: Props) {
  const { t } = useTranslation()
  const alert: Alert | undefined =
    task.alert ?? (task.state === 'ON_HOLD' ? { variant: 'warning', message: t('tasks.onHoldBanner') } : undefined)
  // The task's forms load static data as the task's own manifest lists it.
  const searchServices = useMemo(() => taskSearchServices(task.manifest_version), [task.manifest_version])
  return (
    <div className={`${zoneWidthClass(task)} mx-auto px-4 sm:px-6 lg:px-8 py-10 space-y-8`}>
      <Header task={task} title={title} />
      {alert !== undefined && <AlertBanner alert={alert} />}
      <SearchServiceProvider services={searchServices}>
        {task.view.map((component) => (
          <Zone
            key={`${component.id}:${task.task_id}:${task.state}:${formEpoch}`}
            component={component}
            onAction={onSubmitForm}
          />
        ))}
      </SearchServiceProvider>
      {task.audit && task.audit.length > 0 && <AuditLog entries={task.audit} />}
    </div>
  )
}

function AlertBanner({ alert }: { alert: Alert }) {
  const { title, message, variant } = normaliseAlert(alert)
  return (
    <Banner variant={variant} title={title}>
      {message}
    </Banner>
  )
}

function normaliseAlert(alert: Alert): { message: string; title?: string; variant: AlertVariant } {
  if (typeof alert === 'string') return { message: alert, variant: 'info' }
  return { message: alert.message, title: alert.title, variant: alert.variant ?? 'info' }
}

function AuditLog({ entries }: { entries: AuditEntry[] }) {
  const { t } = useTranslation()
  const sorted = [...entries].sort((a, b) => b.timestamp.localeCompare(a.timestamp))
  return (
    <Surface as="details" elevation="sm" className="group overflow-hidden">
      <summary className="cursor-pointer list-none px-4 py-3 flex items-center justify-between gap-2 hover:bg-app-bg">
        <span className="flex items-center gap-2">
          <svg
            className="w-4 h-4 text-foreground-muted transition-transform group-open:rotate-90"
            viewBox="0 0 20 20"
            fill="currentColor"
          >
            <path
              fillRule="evenodd"
              d="M7.21 14.77a.75.75 0 01.02-1.06L11.168 10 7.23 6.29a.75.75 0 111.04-1.08l4.5 4.25a.75.75 0 010 1.08l-4.5 4.25a.75.75 0 01-1.06-.02z"
              clipRule="evenodd"
            />
          </svg>
          <span className="text-sm font-semibold text-foreground-muted">{t('audit.title')}</span>
          <span className="text-xs text-foreground-muted">{t('audit.entries', { count: entries.length })}</span>
        </span>
      </summary>
      <div className="relative border-t border-border/60 px-4 py-4">
        <div className="absolute left-6.5 top-6 bottom-6 w-px bg-app-surface-muted" aria-hidden />
        <ol className="space-y-4">
          {sorted.map((entry) => (
            <AuditEntryRow key={`${entry.timestamp}:${entry.event}`} entry={entry} />
          ))}
        </ol>
      </div>
    </Surface>
  )
}

function AuditEntryRow({ entry }: { entry: AuditEntry }) {
  const { t } = useTranslation()
  const color = auditDotColor(entry)
  return (
    <li className="relative flex items-start gap-3">
      <span
        className={`relative z-10 mt-1 inline-block w-2.5 h-2.5 rounded-full ring-4 ring-app-surface ${color}`}
        aria-hidden
      />
      <div className="flex-1 min-w-0">
        <div className="flex flex-wrap items-baseline justify-between gap-x-3 gap-y-0.5">
          <p className="text-sm text-foreground">
            <span className="font-semibold">{entry.actor}</span>{' '}
            <span className="text-foreground-muted">{entry.event}</span>
            {entry.from_state && entry.to_state && (
              <span className="ml-2 text-xs text-foreground-muted font-mono">
                {entry.from_state} → {entry.to_state}
              </span>
            )}
          </p>
          <time className="text-xs text-foreground-muted shrink-0" dateTime={entry.timestamp}>
            {formatRelative(entry.timestamp, t)}
          </time>
        </div>
        {entry.details && <p className="mt-1 text-sm text-foreground-muted whitespace-pre-wrap">{entry.details}</p>}
      </div>
    </li>
  )
}

function auditDotColor(entry: AuditEntry): string {
  const text = entry.event.toLowerCase()
  if (text.includes('submit')) return 'bg-success'
  if (text.includes('reject') || text.includes('fail')) return 'bg-error'
  if (text.includes('feedback') || text.includes('clarif') || text.includes('request')) return 'bg-warning'
  if (text.includes('approv')) return 'bg-success'
  return 'bg-foreground-muted'
}

function formatRelative(iso: string, t: TFunction): string {
  const ts = new Date(iso).getTime()
  if (Number.isNaN(ts)) return iso
  const diff = Date.now() - ts
  const mins = Math.floor(diff / 60000)
  if (mins < 1) return t('audit.time.justNow')
  if (mins < 60) return t('audit.time.minutesAgo', { mins })
  const hours = Math.floor(mins / 60)
  if (hours < 24) return t('audit.time.hoursAgo', { hours })
  const days = Math.floor(mins / 60 / 24)
  if (days < 7) return t('audit.time.daysAgo', { days })
  return new Date(iso).toLocaleDateString()
}

function Header({ task, title }: { task: ZoneView; title?: string }) {
  const { t } = useTranslation()
  const statusKey = workflowStatusI18nKey(task.state)
  const statusLabel = statusKey ? t(statusKey) : humanizeStatus(task.state)
  return (
    <div className="flex flex-wrap items-start justify-between gap-x-4 gap-y-2">
      <div className="min-w-0">
        <h1 className="text-2xl sm:text-3xl font-semibold tracking-tight text-foreground">
          {title || humanizeStatus(task.task_type)}
        </h1>
        <p className="mt-1">
          <CopyableId label={t('tasks.taskId')} id={task.task_id} />
        </p>
      </div>
      <span className="shrink-0">
        <StatusBadge tone={workflowStatusTone(task.state)} size="2">
          {statusLabel}
        </StatusBadge>
      </span>
    </div>
  )
}
