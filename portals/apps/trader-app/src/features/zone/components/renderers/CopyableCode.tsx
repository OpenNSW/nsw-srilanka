import { CopyButton } from '@portals/ui'

// A value a template marks copyable, as an inline code pill with a copy button
// inside it. A long value breaks across lines rather than overflow a narrow
// screen.
export function CopyableCode({ value }: { value: string }) {
  return (
    <span className="inline-flex max-w-full items-center gap-1 align-middle bg-app-surface-muted rounded px-1.5 py-0.5">
      <code className="min-w-0 break-all text-primary-strong text-xs font-mono">{value}</code>
      <CopyButton value={value} label={value} />
    </span>
  )
}
