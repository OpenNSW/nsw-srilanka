export function UnknownRenderer({ type }: { type: string }) {
  return (
    <div className="border-2 border-dashed border-amber-200 bg-amber-50/50 px-6 py-10 text-center rounded">
      <p className="text-sm text-amber-700">No renderer for component type: {type}</p>
    </div>
  )
}
