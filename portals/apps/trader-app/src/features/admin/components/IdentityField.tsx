// Workflow ids are often "<name>:<uuid>" composites (node ids are plain definition ids, left whole
// here) — the name is what an admin actually recognizes at a glance; the uuid matters for exact lookups but is unreadable noise inline, so
// it's split off here to be shown smaller/muted with the full id in a title tooltip instead.
function splitIDName(id: string): { name: string; uuid: string | null } {
  const separatorIndex = id.lastIndexOf(':')
  return separatorIndex === -1
    ? { name: id, uuid: null }
    : { name: id.slice(0, separatorIndex), uuid: id.slice(separatorIndex + 1) }
}

// Shared rendering for one labeled workflow/node id (see splitIDName) — used both in the resolve
// view's header and its confirm dialog. Name and uuid render at the same size; de-emphasizing the
// uuid is color's job (text-foreground-muted), not a smaller size next to the name's — a size
// jump between two lines of the same id read as a rendering glitch, not emphasis.
export function IdentityField({ label, id }: { label: string; id: string }) {
  const { name, uuid } = splitIDName(id)
  return (
    <div title={id} className="min-w-0">
      <span className="text-xs text-foreground-muted">{label}</span>
      <div className="font-mono text-sm text-foreground break-all">{name}</div>
      {uuid && <div className="font-mono text-sm text-foreground-muted break-all">{uuid}</div>}
    </div>
  )
}
