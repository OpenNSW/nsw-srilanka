import { StatusBadge } from '@portals/ui'
import type { ConsignmentState } from '@/features/consignment/types'
import { formatState, getStateTone } from '@/features/consignment/utils'

// A consignment's or case's state. size 2 beside its page title.
export function ConsignmentStateBadge({ state, size }: { state: ConsignmentState; size?: '1' | '2' }) {
  return (
    <StatusBadge tone={getStateTone(state)} size={size}>
      {formatState(state)}
    </StatusBadge>
  )
}
