import { Link } from 'react-router-dom'
import type { ConsignmentState, TradeFlow } from '@/features/consignment/types'
import { formatDateTime } from '@/features/consignment/utils'
import { ConsignmentName } from './ConsignmentName'
import { ConsignmentStateBadge } from './ConsignmentStateBadge'
import { TradeFlowBadge } from './TradeFlowBadge'

interface ConsignmentCardProps {
  to: string
  name?: string
  id: string
  // Cases have no trade flow, so their cards leave the badge out.
  flow?: TradeFlow
  state: ConsignmentState
  createdAt?: string
}

// A consignment or case in a list on a narrow screen, where the table's
// columns don't fit: a card that opens it. Its parent is a <ul>.
export function ConsignmentCard({ to, name, id, flow, state, createdAt }: ConsignmentCardProps) {
  return (
    <li>
      <Link
        to={to}
        className="block rounded-xl border border-border bg-app-surface px-4 py-3 transition-colors hover:bg-primary-subtle focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary"
      >
        <ConsignmentName name={name} id={id} />
        <div className="mt-2 flex flex-wrap items-center gap-2">
          {flow && <TradeFlowBadge flow={flow} />}
          <ConsignmentStateBadge state={state} />
        </div>
        <p className="mt-1.5 text-xs text-foreground-muted">{createdAt ? formatDateTime(createdAt) : '-'}</p>
      </Link>
    </li>
  )
}
