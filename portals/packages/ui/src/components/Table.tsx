import type { ReactNode } from 'react'

// A data table that scrolls sideways on a narrow screen. Its rows are a
// LinkRow when each opens a page, or a plain <tr>.
function TableRoot({ children }: { children: ReactNode }) {
  return (
    <div className="overflow-x-auto">
      <table className="w-full">{children}</table>
    </div>
  )
}

// The header row; its children are HeaderCells.
function Header({ children }: { children: ReactNode }) {
  return (
    <thead>
      <tr className="bg-app-surface-muted">{children}</tr>
    </thead>
  )
}

function HeaderCell({ children }: { children: ReactNode }) {
  return (
    <th
      scope="col"
      className="px-6 py-3 text-left text-xs font-semibold uppercase tracking-wider text-foreground-muted"
    >
      {children}
    </th>
  )
}

function Body({ children }: { children: ReactNode }) {
  return <tbody className="divide-y divide-border/60">{children}</tbody>
}

function Cell({ children }: { children: ReactNode }) {
  return <td className="whitespace-nowrap px-6 py-4">{children}</td>
}

export const Table = Object.assign(TableRoot, { Header, HeaderCell, Body, Cell })
