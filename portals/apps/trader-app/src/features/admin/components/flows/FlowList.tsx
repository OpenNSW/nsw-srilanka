import { Badge, Text } from '@radix-ui/themes'
import type { ConsignmentFlow } from '@/features/admin/types'
import { formatDateTime } from '@/features/consignment/utils'
import { humanizeStatus } from '@/utils/formatStatus'

const FLOW_STATUS_COLOR: Record<ConsignmentFlow['status'], 'gray' | 'orange' | 'green'> = {
  STARTING: 'gray',
  STARTED: 'orange',
  COMPLETED: 'green',
}

const FLOW_ROW_GRID = 'grid grid-cols-[1fr_130px_170px_1.5fr] gap-2 items-center'

// A consignment's flows, oldest first. Picking one shows its engine state underneath.
export function FlowList({
  flows,
  selectedId,
  onSelect,
}: {
  flows: ConsignmentFlow[]
  selectedId: string | null
  onSelect: (workflowId: string) => void
}) {
  return (
    <div className="bg-app-surface rounded-lg shadow mb-6 overflow-x-auto">
      <div className={`${FLOW_ROW_GRID} min-w-[700px] border-b border-border text-left`}>
        <div className="p-3 font-medium text-foreground-muted text-sm">Flow</div>
        <div className="p-3 font-medium text-foreground-muted text-sm">Status</div>
        <div className="p-3 font-medium text-foreground-muted text-sm">Started</div>
        <div className="p-3 font-medium text-foreground-muted text-sm">Workflow ID</div>
      </div>
      {flows.length === 0 ? (
        <Text size="2" color="gray" className="block p-3">
          No flows yet.
        </Text>
      ) : (
        flows.map((flow) => {
          const selected = flow.workflowId === selectedId
          return (
            <button
              key={flow.workflowId}
              type="button"
              aria-pressed={selected}
              onClick={() => onSelect(flow.workflowId)}
              className={`${FLOW_ROW_GRID} min-w-[700px] w-full text-left border-b border-border last:border-0 ${
                selected ? 'bg-primary-subtle' : 'hover:bg-app-surface-muted'
              }`}
            >
              <div className="p-3 text-sm text-foreground" title={flow.templateId}>
                {flow.templateName}
              </div>
              <div className="p-3">
                <Badge size="1" color={FLOW_STATUS_COLOR[flow.status]}>
                  {humanizeStatus(flow.status)}
                </Badge>
              </div>
              <div className="p-3 text-xs text-foreground-muted">{formatDateTime(flow.createdAt)}</div>
              <div className="p-3 text-xs font-mono text-foreground-muted break-all">{flow.workflowId}</div>
            </button>
          )
        })
      )}
    </div>
  )
}
