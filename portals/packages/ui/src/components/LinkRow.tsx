import type { ReactNode } from 'react'
import { Link, useNavigate } from 'react-router-dom'

interface LinkRowProps {
  to: string
  children: ReactNode
}

/**
 * A table row that opens `to` when clicked. The row click is only a mouse
 * shortcut: put a {@link RowLink} in one of its cells so keyboard and screen
 * reader users can reach the row too.
 */
export function LinkRow({ to, children }: LinkRowProps) {
  const navigate = useNavigate()
  return (
    <tr onClick={() => void navigate(to)} className="hover:bg-primary-subtle cursor-pointer transition-colors">
      {children}
    </tr>
  )
}

/** The focusable link inside a {@link LinkRow}; the keyboard path to the row's target. */
export function RowLink({ to, children }: LinkRowProps) {
  return (
    <Link
      to={to}
      // The row handles mouse clicks too; stop it navigating a second time.
      onClick={(e) => e.stopPropagation()}
      className="inline-block rounded focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary"
    >
      {children}
    </Link>
  )
}
