import { Button } from '@radix-ui/themes'
import type { Action, ActionVariant, Alert, AlertVariant, AuditEntry, ZoneComponent, ZoneView } from './types'
import { renderZoneComponent } from './renderers'

type Props = {
  task: ZoneView
}

// Zones render in this order when present; any unknown keys render after, in
// insertion order.
const ZONE_ORDER = ['instructions', 'workspace', 'reference']

export function TraderZoneLayout({ task }: Props) {
  const zones = orderedZones(task.view)

  return (
    <div className="max-w-4xl mx-auto px-4 sm:px-6 lg:px-8 py-8 space-y-6">
      <Header task={task} />
      {task.alert !== undefined && <AlertBanner alert={task.alert} />}
      {zones.map(([name, component]) => (
        <ZoneSection
          key={name}
          name={name}
          component={component}
          actions={name === 'workspace' ? task.actions : undefined}
        />
      ))}
      {task.audit && task.audit.length > 0 && <AuditLog entries={task.audit} />}
    </div>
  )
}

function WorkspaceActionBar({ actions }: { actions: Action[] }) {
  return (
    <div className="sticky bottom-0 border-t border-gray-100 bg-white/95 backdrop-blur rounded-b-lg shadow-[0_-4px_12px_-8px_rgba(0,0,0,0.08)]">
      <div className="px-6 py-4 flex justify-end gap-3">
        {actions.map((action) => (
          <ActionButton key={actionKey(action)} action={action} />
        ))}
      </div>
    </div>
  )
}

function ActionButton({ action }: { action: Action }) {
  const variant = action.variant ?? 'primary'
  const handler = () => {
    if (action.kind === 'submit_form') {
      console.log('[zones] submit_form', { command: action.command })
    } else {
      console.log('[zones] task_action', { action: action.action })
    }
  }
  return (
    <Button onClick={handler} size="3" variant={buttonVariant(variant)} color={buttonColor(variant)}>
      {action.label}
    </Button>
  )
}

function buttonVariant(v: ActionVariant): 'solid' | 'outline' {
  return v === 'outline' ? 'outline' : 'solid'
}

function buttonColor(v: ActionVariant): 'red' | undefined {
  return v === 'danger' ? 'red' : undefined
}

function actionKey(action: Action): string {
  return action.kind === 'submit_form' ? `submit:${action.command}` : `action:${action.action}`
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
        container: 'bg-emerald-50 border-emerald-200',
        icon: 'text-emerald-500',
        title: 'text-emerald-800',
        body: 'text-emerald-900',
      }
    case 'warning':
      return {
        container: 'bg-amber-50 border-amber-300',
        icon: 'text-amber-500',
        title: 'text-amber-800',
        body: 'text-amber-900',
      }
    case 'error':
      return {
        container: 'bg-red-50 border-red-300',
        icon: 'text-red-500',
        title: 'text-red-800',
        body: 'text-red-900',
      }
    case 'info':
    default:
      return {
        container: 'bg-sky-50 border-sky-200',
        icon: 'text-sky-500',
        title: 'text-sky-800',
        body: 'text-sky-900',
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
  const sorted = [...entries].sort((a, b) => b.timestamp.localeCompare(a.timestamp))
  return (
    <details className="group rounded-lg border border-gray-200 bg-white overflow-hidden">
      <summary className="cursor-pointer list-none px-4 py-3 flex items-center justify-between gap-2 hover:bg-gray-50">
        <span className="flex items-center gap-2">
          <svg
            className="w-4 h-4 text-gray-400 transition-transform group-open:rotate-90"
            viewBox="0 0 20 20"
            fill="currentColor"
          >
            <path
              fillRule="evenodd"
              d="M7.21 14.77a.75.75 0 01.02-1.06L11.168 10 7.23 6.29a.75.75 0 111.04-1.08l4.5 4.25a.75.75 0 010 1.08l-4.5 4.25a.75.75 0 01-1.06-.02z"
              clipRule="evenodd"
            />
          </svg>
          <span className="text-sm font-semibold text-gray-700">Activity</span>
          <span className="text-xs text-gray-400">· {entries.length} entries</span>
        </span>
      </summary>
      <div className="relative border-t border-gray-100 px-4 py-4">
        <div className="absolute left-[26px] top-6 bottom-6 w-px bg-gray-200" aria-hidden />
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
  const color = auditDotColor(entry)
  return (
    <li className="relative flex items-start gap-3">
      <span
        className={`relative z-10 mt-1 inline-block w-2.5 h-2.5 rounded-full ring-4 ring-white ${color}`}
        aria-hidden
      />
      <div className="flex-1 min-w-0">
        <div className="flex items-baseline justify-between gap-3">
          <p className="text-sm text-gray-800">
            <span className="font-semibold">{entry.actor}</span> <span className="text-gray-600">{entry.event}</span>
            {entry.from_state && entry.to_state && (
              <span className="ml-2 text-xs text-gray-400 font-mono">
                {entry.from_state} → {entry.to_state}
              </span>
            )}
          </p>
          <time className="text-xs text-gray-400 shrink-0" dateTime={entry.timestamp}>
            {formatRelative(entry.timestamp)}
          </time>
        </div>
        {entry.details && <p className="mt-1 text-sm text-gray-500 whitespace-pre-wrap">{entry.details}</p>}
      </div>
    </li>
  )
}

function auditDotColor(entry: AuditEntry): string {
  const text = entry.event.toLowerCase()
  if (text.includes('submit')) return 'bg-emerald-500'
  if (text.includes('reject') || text.includes('fail')) return 'bg-red-500'
  if (text.includes('feedback') || text.includes('clarif') || text.includes('request')) return 'bg-amber-500'
  if (text.includes('approv')) return 'bg-emerald-500'
  return 'bg-gray-400'
}

function formatRelative(iso: string): string {
  const ts = new Date(iso).getTime()
  if (Number.isNaN(ts)) return iso
  const diff = Date.now() - ts
  const mins = Math.floor(diff / 60000)
  if (mins < 1) return 'just now'
  if (mins < 60) return `${mins}m ago`
  const hours = Math.floor(mins / 60)
  if (hours < 24) return `${hours}h ago`
  const days = Math.floor(hours / 24)
  if (days < 7) return `${days}d ago`
  return new Date(iso).toLocaleDateString()
}

function Header({ task }: { task: ZoneView }) {
  return (
    <div className="border-b border-gray-200 pb-4">
      <div className="flex items-center justify-between gap-4">
        <div>
          <h1 className="text-2xl font-bold text-gray-900">{task.task_type}</h1>
          <p className="text-xs text-gray-500 mt-1 font-mono">{task.task_id}</p>
        </div>
        <span className="inline-flex items-center rounded-full bg-indigo-50 px-3 py-1 text-xs font-semibold text-indigo-700 border border-indigo-100">
          {task.state}
        </span>
      </div>
    </div>
  )
}

function ZoneSection({ name, component, actions }: { name: string; component: ZoneComponent; actions?: Action[] }) {
  return (
    <section className="space-y-2">
      <div className="flex items-center gap-2">
        <span className="text-xs font-semibold uppercase tracking-widest text-gray-400">{name}</span>
        <span className="text-[10px] font-medium uppercase tracking-wider text-gray-300">{component.type}</span>
      </div>
      <div className="bg-white rounded-lg shadow-sm border border-gray-100">
        <div className="p-6">{renderZoneComponent(component)}</div>
        {actions && actions.length > 0 && <WorkspaceActionBar actions={actions} />}
      </div>
    </section>
  )
}
