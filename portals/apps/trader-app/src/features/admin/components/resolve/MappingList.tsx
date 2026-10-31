import { Section } from '@/features/admin/components/resolve/Section'

// One of a parked node's mappings as "from → to" rows, sorted so the list is stable across
// refreshes. A trailing "?" on a key marks it optional in the node's definition; it is shown as a
// note rather than as part of the name, since the name is what an admin would type into a patch.
export function MappingList({
  title,
  note,
  mapping,
}: {
  title: string
  note: string
  mapping: Record<string, string>
}) {
  const rows = Object.entries(mapping).sort(([a], [b]) => a.localeCompare(b))
  return (
    <Section title={title} note={note}>
      <div className="bg-app-surface-muted rounded p-3 text-xs font-mono overflow-auto max-h-48">
        {rows.map(([rawKey, value]) => {
          const optional = rawKey.endsWith('?')
          const key = optional ? rawKey.slice(0, -1) : rawKey
          return (
            <div key={rawKey} className="whitespace-pre-wrap break-all py-0.5">
              {key}
              {optional && <span className="text-foreground-muted"> (optional)</span>} → {value}
            </div>
          )
        })}
      </div>
    </Section>
  )
}
