// How many items a list holds, next to its title.
export function CountBadge({ count }: { count: number }) {
  return (
    <span className="inline-flex items-center rounded-full bg-primary-subtle px-2.5 py-0.5 text-sm font-medium text-primary-strong">
      {count}
    </span>
  )
}
