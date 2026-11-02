import { render, screen } from '@testing-library/react'
import { Theme } from '@radix-ui/themes'
import { describe, expect, it } from 'vitest'
import { NodeRow } from '@/features/admin/components/engine/NodeRow'
import { NO_EXPAND_SIGNAL } from '@/features/admin/components/engine/shared'
import type { EngineNode } from '@/features/admin/types'

const parked: EngineNode = {
  id: 'pay_fee',
  step_id: 'step-1',
  type: 'TASK',
  status: 'AWAITING_ADMIN',
  created_at: '2026-01-01T00:00:00Z',
  updated_at: '2026-01-01T00:00:00Z',
}
const waiting: EngineNode = { ...parked, id: 'review', status: 'RUNNING' }

function renderRow(node: EngineNode, canIntervene: boolean) {
  render(
    <Theme>
      <NodeRow
        node={node}
        depth={0}
        workflowId="wf-1"
        workflowKind="workflow"
        edges={[]}
        showAllNodes={false}
        expandSignal={NO_EXPAND_SIGNAL}
        onOpenVariables={() => undefined}
        onOpenResolve={() => undefined}
        canIntervene={canIntervene}
        onRefresh={() => undefined}
      />
    </Theme>,
  )
}

describe('NodeRow', () => {
  it('offers Resolve on a parked node and Force park on a waiting task to an admin who may intervene', () => {
    renderRow(parked, true)
    expect(screen.getByRole('button', { name: 'Resolve' })).toBeTruthy()

    renderRow(waiting, true)
    expect(screen.getByRole('button', { name: 'Force park' })).toBeTruthy()
  })

  it('only reports to an admin who may not intervene', () => {
    renderRow(parked, false)
    renderRow(waiting, false)

    expect(screen.queryByRole('button', { name: 'Resolve' })).toBeNull()
    expect(screen.queryByRole('button', { name: 'Force park' })).toBeNull()
  })
})
