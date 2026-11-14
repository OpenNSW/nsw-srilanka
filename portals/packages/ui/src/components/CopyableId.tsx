import { CopyButton } from './CopyButton'

// A labelled ID with a copy button, for the IDs traders and support quote.
export function CopyableId({ label, id }: { label: string; id: string }) {
  // The ID breaks across lines rather than overflow a narrow screen, so all of
  // it stays readable.
  return (
    <span className="inline-flex max-w-full items-center gap-1 text-xs text-foreground-muted">
      <span className="shrink-0">{label}</span>
      <span className="min-w-0 break-all font-mono">{id}</span>
      <CopyButton value={id} label={label} />
    </span>
  )
}
