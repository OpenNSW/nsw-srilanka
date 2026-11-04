import { fireEvent, render, screen } from '@testing-library/react'
import { Theme } from '@radix-ui/themes'
import { describe, expect, it, vi } from 'vitest'
import { ResolveAdminInterventionView } from '@/features/admin/components/resolve/ResolveAdminInterventionView'
import type { AdminResolutionTarget } from '@/features/admin/components/engine/shared'

// config.js sets this in the app; @/constants reads it as it loads.
vi.hoisted(() => {
  window.__APP_CONFIG__ = { API_BASE_URL: 'http://api.test' }
})

const target: AdminResolutionTarget = {
  workflowId: 'wf-1',
  workflowKind: 'workflow',
  nodeId: 'pay_fee',
  stepId: 'step-1',
  isGateway: false,
  onResolved: () => undefined,
}

function renderView(overrides: Partial<AdminResolutionTarget> = {}) {
  render(
    <Theme>
      <ResolveAdminInterventionView target={{ ...target, ...overrides }} onBack={() => undefined} />
    </Theme>,
  )
}

describe('ResolveAdminInterventionView', () => {
  it('shows what the chosen action does under the action buttons', () => {
    renderView()
    expect(screen.getByText('Select an action to see what it does.')).toBeTruthy()

    fireEvent.click(screen.getByRole('button', { name: 'Retry' }))
    expect(screen.getByText(/Re-runs the node for real/)).toBeTruthy()
    expect(screen.getByRole('button', { name: 'Retry' }).getAttribute('aria-pressed')).toBe('true')

    fireEvent.click(screen.getByRole('button', { name: 'Abort' }))
    expect(screen.getByText(/Fails this node and the whole workflow/)).toBeTruthy()
    expect(screen.queryByText(/Re-runs the node for real/)).toBeNull()
    expect(screen.getByRole('button', { name: 'Retry' }).getAttribute('aria-pressed')).toBe('false')
  })

  it('gives each action a focusable button that explains it', () => {
    renderView()
    expect(screen.getByRole('button', { name: 'What Retry does' })).toBeTruthy()
    expect(screen.getByRole('button', { name: 'What Complete does' })).toBeTruthy()
    expect(screen.getByRole('button', { name: 'What Abort does' })).toBeTruthy()
  })

  it('disables Complete on a gateway node', () => {
    renderView({ isGateway: true })
    expect(screen.getByRole<HTMLButtonElement>('button', { name: 'Complete' }).disabled).toBe(true)
    expect(screen.getByRole<HTMLButtonElement>('button', { name: 'Retry' }).disabled).toBe(false)
  })
})
