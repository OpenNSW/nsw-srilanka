import { Theme } from '@radix-ui/themes'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { ForceParkDialog } from '@/features/admin/components/engine/ForceParkDialog'
import { forceParkNode } from '@/features/admin/service'

vi.mock('@/features/admin/service', () => ({
  getConsignmentForAdmin: vi.fn(),
  getCaseForAdmin: vi.fn(),
  getWorkflowEngineStatus: vi.fn(),
  getTaskWorkflowEngineStatus: vi.fn(),
  forceParkNode: vi.fn(),
  resolveAdminIntervention: vi.fn(),
}))

const mockForcePark = vi.mocked(forceParkNode)

function renderDialog() {
  const onOpenChange = vi.fn()
  const onParked = vi.fn()
  render(
    <Theme>
      <ForceParkDialog
        open
        onOpenChange={onOpenChange}
        workflowId="wf-1"
        workflowKind="task"
        nodeId="pay_fee"
        stepId="step-1"
        onParked={onParked}
      />
    </Theme>,
  )
  return { onOpenChange, onParked }
}

function enterReasonAndConfirm(reason: string) {
  fireEvent.change(screen.getByRole('textbox', { name: 'Reason' }), { target: { value: reason } })
  fireEvent.click(screen.getByRole('button', { name: 'Force park' }))
}

describe('ForceParkDialog', () => {
  afterEach(() => {
    vi.resetAllMocks()
    vi.restoreAllMocks()
  })

  it('needs a reason before it can force-park', () => {
    renderDialog()
    expect(screen.getByRole<HTMLButtonElement>('button', { name: 'Force park' }).disabled).toBe(true)
  })

  it('force-parks the step, closes and asks the caller to refresh', async () => {
    mockForcePark.mockResolvedValue(undefined)
    const { onOpenChange, onParked } = renderDialog()

    enterReasonAndConfirm('callback never arrived')

    await waitFor(() => expect(onParked).toHaveBeenCalledTimes(1))
    expect(mockForcePark).toHaveBeenCalledWith('wf-1', 'step-1', 'callback never arrived', 'task')
    expect(onOpenChange).toHaveBeenCalledWith(false)
  })

  it('stays open with the error when force-parking fails', async () => {
    vi.spyOn(console, 'error').mockImplementation(() => undefined)
    mockForcePark.mockRejectedValue(new Error('node is not running'))
    const { onOpenChange, onParked } = renderDialog()

    enterReasonAndConfirm('callback never arrived')

    expect(await screen.findByText('node is not running')).toBeTruthy()
    expect(onParked).not.toHaveBeenCalled()
    expect(onOpenChange).not.toHaveBeenCalledWith(false)
  })
})
