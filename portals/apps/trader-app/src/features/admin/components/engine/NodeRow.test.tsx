import { render, screen } from '@testing-library/react'
import { Theme } from '@radix-ui/themes'
import { describe, expect, it, vi } from 'vitest'
import { NodeRow } from '@/features/admin/components/engine/NodeRow'
import { NO_EXPAND_SIGNAL } from '@/features/admin/components/engine/shared'
import type { EngineNode } from '@/features/admin/types'

// config.js sets this in the app; @/constants reads it as it loads.
vi.hoisted(() => {
  window.__APP_CONFIG__ = { API_BASE_URL: 'http://api.test' }
})

const auth = vi.hoisted(() => ({ scope: '' }))
vi.mock('react-oidc-context', () => ({ useAuth: () => ({ user: { scope: auth.scope } }) }))

const parked: EngineNode = {
  id: 'pay_fee',
  step_id: 'step-1',
  type: 'TASK',
  status: 'AWAITING_ADMIN',
  created_at: '2026-01-01T00:00:00Z',
  updated_at: '2026-01-01T00:00:00Z',
}
const waiting: EngineNode = { ...parked, id: 'review', status: 'RUNNING' }

function renderRow(node: EngineNode) {
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
        onRefresh={() => undefined}
      />
    </Theme>,
  )
}

describe('NodeRow', () => {
  it('offers Resolve on a parked node and Force park on a waiting task to an admin who may intervene', () => {
    auth.scope = 'openid consignment:adminread consignment:adminwrite'
    renderRow(parked)
    expect(screen.getByRole('button', { name: 'Resolve' })).toBeTruthy()

    renderRow(waiting)
    expect(screen.getByRole('button', { name: 'Force park' })).toBeTruthy()
  })

  it('only reports to an admin who may not intervene', () => {
    auth.scope = 'openid consignment:adminread'
    renderRow(parked)
    renderRow(waiting)

    expect(screen.queryByRole('button', { name: 'Resolve' })).toBeNull()
    expect(screen.queryByRole('button', { name: 'Force park' })).toBeNull()
  })
})
