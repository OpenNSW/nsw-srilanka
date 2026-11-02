import type { ReactNode } from 'react'
import { CountBadge } from './CountBadge'

interface PageHeaderProps {
  title: string
  // Shown beside the title once there is at least one item.
  count?: number
  actions?: ReactNode
}

// A list page's heading: its title, how many items it holds, and its actions.
export function PageHeader({ title, count, actions }: PageHeaderProps) {
  return (
    <div className="mb-6 flex flex-wrap items-center justify-between gap-4">
      <div className="flex items-center gap-3">
        <h1 className="text-2xl font-semibold tracking-tight text-foreground">{title}</h1>
        {count !== undefined && count > 0 && <CountBadge count={count} />}
      </div>
      {actions}
    </div>
  )
}
