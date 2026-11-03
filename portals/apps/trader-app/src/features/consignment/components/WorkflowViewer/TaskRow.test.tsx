import { render, screen } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { Theme } from '@radix-ui/themes'
import { describe, expect, it, vi } from 'vitest'
import '@/i18n'
import { TaskRow } from '@/features/consignment/components/WorkflowViewer/TaskRow'
import type { WorkflowNode } from '@/features/consignment/types'

// config.js sets this in the app; @/constants reads it as it loads.
vi.hoisted(() => {
  window.__APP_CONFIG__ = { API_BASE_URL: 'http://api.test' }
})

const auth = vi.hoisted(() => ({ scope: '' }))
vi.mock('react-oidc-context', () => ({ useAuth: () => ({ user: { scope: auth.scope } }) }))

const step: WorkflowNode = {
  id: 'task-1',
  createdAt: '2026-01-01T00:00:00Z',
  updatedAt: '2026-01-01T00:00:00Z',
  workflowNodeTemplate: { name: 'Pay the fee', description: '', type: 'SIMPLE_FORM' },
  state: 'READY',
  depends_on: [],
}

function renderRow(scope: string) {
  auth.scope = scope
  render(
    <Theme>
      <MemoryRouter>
        <ul>
          <TaskRow step={step} consignmentId="c-1" needsAction />
        </ul>
      </MemoryRouter>
    </Theme>,
  )
}

describe('TaskRow', () => {
  it('offers Continue on a task that needs action to a user who may act on it', () => {
    renderRow('openid task:read task:write')

    expect(screen.getByRole('link', { name: 'Continue Pay the fee' })).toBeTruthy()
  })

  it('offers View, not Continue, to a user who may only open it', () => {
    renderRow('openid task:read')

    expect(screen.getByRole('link', { name: 'View Pay the fee' })).toBeTruthy()
    expect(screen.queryByRole('link', { name: 'Continue Pay the fee' })).toBeNull()
  })

  it('links nowhere for a user who may not open it', () => {
    renderRow('openid task:write')

    expect(screen.queryByRole('link')).toBeNull()
  })
})
