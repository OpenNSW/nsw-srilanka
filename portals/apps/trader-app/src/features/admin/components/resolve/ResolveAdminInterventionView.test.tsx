import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { Theme } from '@radix-ui/themes'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { ResolveAdminInterventionView } from '@/features/admin/components/resolve/ResolveAdminInterventionView'
import type { AdminResolutionTarget } from '@/features/admin/components/engine/shared'
import { resolveAdminIntervention } from '@/features/admin/service'

// config.js sets this in the app; @/constants reads it as it loads.
vi.hoisted(() => {
  window.__APP_CONFIG__ = { API_BASE_URL: 'http://api.test' }
})

vi.mock('@/features/admin/service', () => ({
  getConsignmentForAdmin: vi.fn(),
  getCaseForAdmin: vi.fn(),
  getWorkflowEngineStatus: vi.fn(),
  getTaskWorkflowEngineStatus: vi.fn(),
  forceParkNode: vi.fn(),
  resolveAdminIntervention: vi.fn(),
}))

const mockResolve = vi.mocked(resolveAdminIntervention)

const target: AdminResolutionTarget = {
  workflowId: 'wf-1',
  workflowKind: 'workflow',
  nodeId: 'pay_fee',
  stepId: 'step-1',
  isGateway: false,
  onResolved: () => undefined,
}

function renderView(overrides: Partial<AdminResolutionTarget> = {}, onBack: () => void = () => undefined) {
  render(
    <Theme>
      <ResolveAdminInterventionView target={{ ...target, ...overrides }} onBack={onBack} />
    </Theme>,
  )
}

// Picks Abort, which sends no variables patch, gives a reason and confirms it.
function abortWithReason(reason: string) {
  fireEvent.click(screen.getByRole('button', { name: 'Abort' }))
  fireEvent.change(screen.getByRole('textbox', { name: 'Reason' }), { target: { value: reason } })
  fireEvent.click(screen.getByRole('button', { name: 'Submit' }))
  fireEvent.click(screen.getByRole('button', { name: 'Yes, abort' }))
}

describe('ResolveAdminInterventionView', () => {
  afterEach(() => {
    vi.resetAllMocks()
    vi.restoreAllMocks()
  })

  it('resolves the node, then refreshes its workflow and goes back', async () => {
    mockResolve.mockResolvedValue(undefined)
    const onResolved = vi.fn()
    const onBack = vi.fn()
    renderView({ onResolved }, onBack)

    abortWithReason('cannot continue')

    await waitFor(() => expect(onBack).toHaveBeenCalledTimes(1))
    expect(onResolved).toHaveBeenCalledTimes(1)
    expect(mockResolve).toHaveBeenCalledWith(
      'wf-1',
      'step-1',
      { action: 'ABORT', global_variables_patch: undefined, reason: 'cannot continue' },
      'workflow',
    )
  })

  it('stays on the screen with the error when resolving fails', async () => {
    vi.spyOn(console, 'error').mockImplementation(() => undefined)
    mockResolve.mockRejectedValue(new Error('node re-parked'))
    const onResolved = vi.fn()
    const onBack = vi.fn()
    renderView({ onResolved }, onBack)

    abortWithReason('cannot continue')

    expect(await screen.findByText('node re-parked')).toBeTruthy()
    expect(onResolved).not.toHaveBeenCalled()
    expect(onBack).not.toHaveBeenCalled()
    expect(screen.getByRole<HTMLButtonElement>('button', { name: 'Submit' }).disabled).toBe(false)
  })

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

  it('gives the error, the cached result and the workflow variables a copy button', () => {
    const writeText = vi.fn().mockResolvedValue(undefined)
    Object.defineProperty(navigator, 'clipboard', { value: { writeText }, configurable: true })
    const globalVariables = { fcau: { reference_number: '034/25236' } }
    renderView({ lastError: 'activity timed out', cachedTaskResult: { payment_status: 'PAID' }, globalVariables })

    expect(screen.getByRole('button', { name: 'Copy Last error' })).toBeTruthy()
    expect(screen.getByRole('button', { name: 'Copy Cached task result' })).toBeTruthy()
    fireEvent.click(screen.getByRole('button', { name: 'Copy Workflow variables' }))
    expect(writeText).toHaveBeenCalledWith(JSON.stringify(globalVariables, null, 2))

    Object.defineProperty(navigator, 'clipboard', { value: undefined, configurable: true })
  })
})
