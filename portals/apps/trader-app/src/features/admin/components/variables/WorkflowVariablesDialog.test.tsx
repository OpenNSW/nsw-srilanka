import { fireEvent, render, screen } from '@testing-library/react'
import { Theme } from '@radix-ui/themes'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { WorkflowVariablesDialog } from '@/features/admin/components/variables/WorkflowVariablesDialog'
import type { WorkflowVariablesTarget } from '@/features/admin/components/engine/shared'

function renderDialog(target: WorkflowVariablesTarget | null) {
  render(
    <Theme>
      <WorkflowVariablesDialog target={target} onClose={() => undefined} />
    </Theme>,
  )
}

describe('WorkflowVariablesDialog', () => {
  afterEach(() => {
    Object.defineProperty(navigator, 'clipboard', { value: undefined, configurable: true })
  })

  it('shows the variables with a button that copies them', () => {
    const writeText = vi.fn().mockResolvedValue(undefined)
    Object.defineProperty(navigator, 'clipboard', { value: { writeText }, configurable: true })
    const variables = { fcau: { reference_number: '034/25236' } }
    renderDialog({ workflowId: 'wf-1', label: 'Task workflow', variables })

    expect(screen.getByText('Task workflow global variables')).toBeTruthy()
    expect(screen.getByText(/"reference_number": "034\/25236"/)).toBeTruthy()
    fireEvent.click(screen.getByRole('button', { name: 'Copy Task workflow global variables' }))
    expect(writeText).toHaveBeenCalledWith(JSON.stringify(variables, null, 2))
  })

  it('says so when the workflow has no variables, with nothing to copy', () => {
    Object.defineProperty(navigator, 'clipboard', { value: { writeText: vi.fn() }, configurable: true })
    renderDialog({ workflowId: 'wf-1', label: 'Task workflow', variables: {} })

    expect(screen.getByText('No global variables recorded for this workflow.')).toBeTruthy()
    expect(screen.queryByRole('button', { name: /^Copy/ })).toBeNull()
  })

  it('is closed with no target', () => {
    renderDialog(null)

    expect(screen.queryByRole('dialog')).toBeNull()
  })
})
