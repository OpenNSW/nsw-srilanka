import { Section } from '@/features/admin/components/resolve/Section'

// keyParts splits a mapping key as the workflow engine reads it: "x?!" is optional and removes
// the destination when its source is absent, "x?" is optional and keeps the destination, and a
// key with neither is required. The marker is shown as a note rather than as part of the name,
// since the name is what an admin would type into a patch.
function keyParts(rawKey: string): { key: string; note?: string } {
  if (rawKey.endsWith('?!')) return { key: rawKey.slice(0, -2), note: 'optional, removed when absent' }
  if (rawKey.endsWith('?')) return { key: rawKey.slice(0, -1), note: 'optional' }
  return { key: rawKey }
}

// One of a parked node's mappings as "from → to" rows, sorted so the list is stable across
// refreshes.
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
          const { key, note: keyNote } = keyParts(rawKey)
          return (
            <div key={rawKey} className="whitespace-pre-wrap break-all py-0.5">
              {key}
              {keyNote && <span className="text-foreground-muted"> ({keyNote})</span>} → {value}
            </div>
          )
        })}
      </div>
    </Section>
  )
}
