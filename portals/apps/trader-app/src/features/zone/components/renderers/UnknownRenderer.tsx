export function UnknownRenderer({ type }: { type: string }) {
  return (
    <div className="border-2 border-dashed border-warning-subtle bg-warning-subtle/50 px-6 py-10 text-center rounded">
      <p className="text-sm text-warning-strong">No renderer for component type: {type}</p>
    </div>
  )
}
