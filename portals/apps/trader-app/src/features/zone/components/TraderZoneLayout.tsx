import type { Alert, AlertVariant, AuditEntry, ZoneComponent, ZoneView } from '@/features/zone/types'
import { useTranslation } from 'react-i18next'
import type { TFunction } from 'i18next'
import { Zone } from './Zone'

type Props = {
  task: ZoneView
  onSubmitForm?: (command: string, data: Record<string, unknown>) => Promise<void>
}

const ZONE_ORDER = ['instructions', 'workspace', 'reference']

export function TraderZoneLayout({ task, onSubmitForm }: Props) {
  const zones = orderedZones(task.view)

  return (
    <div className="max-w-6xl mx-auto px-4 sm:px-6 lg:px-8 py-8 space-y-6">
      <Header task={task} />
      {task.alert !== undefined && <AlertBanner alert={task.alert} />}
      {zones.map(([name, component]) => (
        <Zone key={`${name}:${task.task_id}:${task.state}`} name={name} component={component} onAction={onSubmitForm} />
      ))}
      {task.audit && task.audit.length > 0 && <AuditLog entries={task.audit} />}
    </div>
  )
}

function orderedZones(view: Record<string, ZoneComponent>): Array<[string, ZoneComponent]> {
  const known = ZONE_ORDER.filter((k) => k in view).map((k) => [k, view[k]] as [string, ZoneComponent])
  const extras = Object.entries(view).filter(([k]) => !ZONE_ORDER.includes(k))
  return [...known, ...extras]
}

function AlertBanner({ alert }: { alert: Alert }) {
  const { title, message, variant } = normaliseAlert(alert)
  const styles = alertStyles(variant)
  return (
    <div className={`rounded-lg border p-4 flex items-start gap-3 ${styles.container}`}>
      <span className={`mt-0.5 ${styles.icon}`}>{alertIcon(variant)}</span>
      <div className="flex-1 min-w-0">
        {title && <p className={`text-sm font-semibold ${styles.title}`}>{title}</p>}
        <p className={`text-sm whitespace-pre-wrap ${styles.body}`}>{message}</p>
      </div>
    </div>
  )
}

function normaliseAlert(alert: Alert): { message: string; title?: string; variant: AlertVariant } {
  if (typeof alert === 'string') return { message: alert, variant: 'info' }
  return { message: alert.message, title: alert.title, variant: alert.variant ?? 'info' }
}

function alertStyles(variant: AlertVariant) {
  switch (variant) {
    case 'success':
      return {
        container: 'bg-success-subtle border-success-subtle',
        icon: 'text-success',
        title: 'text-success-strong',
        body: 'text-success-strong',
      }
    case 'warning':
      return {
        container: 'bg-warning-subtle border-warning-subtle',
        icon: 'text-warning',
        title: 'text-warning-strong',
        body: 'text-warning-strong',
      }
    case 'error':
      return {
        container: 'bg-error-subtle border-error-subtle',
        icon: 'text-error',
        title: 'text-error-strong',
        body: 'text-error-strong',
      }
    case 'info':
    default:
      return {
        container: 'bg-info-subtle border-info-subtle',
        icon: 'text-info',
        title: 'text-info-strong',
        body: 'text-info-strong',
      }
  }
}

function alertIcon(variant: AlertVariant) {
  if (variant === 'success') {
    return (
      <svg xmlns="http://www.w3.org/2000/svg" className="w-5 h-5" viewBox="0 0 20 20" fill="currentColor">
        <path
          fillRule="evenodd"
          d="M16.707 5.293a1 1 0 010 1.414l-8 8a1 1 0 01-1.414 0l-4-4a1 1 0 011.414-1.414L8 12.586l7.293-7.293a1 1 0 011.414 0z"
          clipRule="evenodd"
        />
      </svg>
    )
  }
  return (
    <svg xmlns="http://www.w3.org/2000/svg" className="w-5 h-5" viewBox="0 0 20 20" fill="currentColor">
      <path
        fillRule="evenodd"
        d="M8.257 3.099c.765-1.36 2.722-1.36 3.486 0l5.58 9.92c.75 1.334-.213 2.98-1.742 2.98H4.42c-1.53 0-2.493-1.646-1.743-2.98l5.58-9.92zM11 13a1 1 0 11-2 0 1 1 0 012 0zm-1-8a1 1 0 00-1 1v3a1 1 0 002 0V6a1 1 0 00-1-1z"
        clipRule="evenodd"
      />
    </svg>
  )
}

function AuditLog({ entries }: { entries: AuditEntry[] }) {
  const { t } = useTranslation()
  const sorted = [...entries].sort((a, b) => b.timestamp.localeCompare(a.timestamp))
  return (
    <details className="group rounded-lg border border-border bg-background overflow-hidden">
      <summary className="cursor-pointer list-none px-4 py-3 flex items-center justify-between gap-2 hover:bg-surface">
        <span className="flex items-center gap-2">
          <svg
            className="w-4 h-4 text-foreground-subtle transition-transform group-open:rotate-90"
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
          <span className="text-xs text-foreground-subtle">{t('audit.entries', { count: entries.length })}</span>
        </span>
      </summary>
      <div className="relative border-t border-border px-4 py-4">
        <div className="absolute left-6.5 top-6 bottom-6 w-px bg-surface-muted" aria-hidden />
        <ol className="space-y-4">
          {sorted.map((entry) => (
            <AuditEntryRow key={`${entry.timestamp}:${entry.event}`} entry={entry} />
          ))}
        </ol>
      </div>
    </details>
  )
}

function AuditEntryRow({ entry }: { entry: AuditEntry }) {
  const { t } = useTranslation()
  const color = auditDotColor(entry)
  return (
    <li className="relative flex items-start gap-3">
      <span
        className={`relative z-10 mt-1 inline-block w-2.5 h-2.5 rounded-full ring-4 ring-white ${color}`}
        aria-hidden
      />
      <div className="flex-1 min-w-0">
        <div className="flex items-baseline justify-between gap-3">
          <p className="text-sm text-foreground">
            <span className="font-semibold">{entry.actor}</span>{' '}
            <span className="text-foreground-muted">{entry.event}</span>
            {entry.from_state && entry.to_state && (
              <span className="ml-2 text-xs text-foreground-subtle font-mono">
                {entry.from_state} → {entry.to_state}
              </span>
            )}
          </p>
          <time className="text-xs text-foreground-subtle shrink-0" dateTime={entry.timestamp}>
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
  return 'bg-foreground-subtle'
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

function Header({ task }: { task: ZoneView }) {
  return (
    <div className="border-b border-border pb-4">
      <div className="flex items-center justify-between gap-4">
        <div>
          <h1 className="text-2xl font-bold text-foreground">{task.task_type}</h1>
          <p className="text-xs text-foreground-muted mt-1 font-mono">{task.task_id}</p>
        </div>
        <span className="inline-flex items-center rounded-full bg-primary-subtle px-3 py-1 text-xs font-semibold text-primary border border-primary-subtle">
          {task.state}
        </span>
      </div>
    </div>
  )
}
