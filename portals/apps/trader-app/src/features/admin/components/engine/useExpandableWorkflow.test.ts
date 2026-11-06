import { act, renderHook, waitFor } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { useExpandableWorkflow } from '@/features/admin/components/engine/useExpandableWorkflow'
import { NO_EXPAND_SIGNAL, type ExpandSignal } from '@/features/admin/components/engine/shared'
import { getTaskWorkflowEngineStatus, getWorkflowEngineStatus } from '@/features/admin/service'
import type { EngineStatus } from '@/features/admin/types'

vi.mock('@/features/admin/service', () => ({
  getConsignmentForAdmin: vi.fn(),
  getCaseForAdmin: vi.fn(),
  getWorkflowEngineStatus: vi.fn(),
  getTaskWorkflowEngineStatus: vi.fn(),
  forceParkNode: vi.fn(),
  resolveAdminIntervention: vi.fn(),
}))

const mockGetWorkflowStatus = vi.mocked(getWorkflowEngineStatus)
const mockGetTaskStatus = vi.mocked(getTaskWorkflowEngineStatus)

function makeStatus(workflowId: string): EngineStatus {
  return { workflow_id: workflowId, status: 'RUNNING', nodes: [], audit_trail: [] }
}

function renderBranch(workflowId: string, kind: 'child' | 'task', expandSignal: ExpandSignal = NO_EXPAND_SIGNAL) {
  return renderHook(({ signal }) => useExpandableWorkflow(workflowId, kind, signal), {
    initialProps: { signal: expandSignal },
  })
}

describe('useExpandableWorkflow', () => {
  afterEach(() => {
    vi.resetAllMocks()
  })

  it('loads nothing until the branch is first expanded', async () => {
    mockGetWorkflowStatus.mockResolvedValue(makeStatus('child-1'))

    const { result } = renderBranch('child-1', 'child')
    expect(result.current.expanded).toBe(false)
    expect(result.current.loading).toBe(false)
    expect(mockGetWorkflowStatus).not.toHaveBeenCalled()

    act(() => result.current.toggle())

    expect(result.current.expanded).toBe(true)
    expect(result.current.loading).toBe(true)
    await waitFor(() => expect(result.current.status?.workflow_id).toBe('child-1'))
    expect(result.current.loading).toBe(false)
    expect(mockGetWorkflowStatus).toHaveBeenCalledWith('child-1', expect.any(AbortSignal))
  })

  it('reads a task workflow through the task route', async () => {
    mockGetTaskStatus.mockResolvedValue(makeStatus('tw-1'))

    const { result } = renderBranch('tw-1', 'task')
    act(() => result.current.toggle())

    await waitFor(() => expect(result.current.status?.workflow_id).toBe('tw-1'))
    expect(mockGetTaskStatus).toHaveBeenCalledTimes(1)
    expect(mockGetWorkflowStatus).not.toHaveBeenCalled()
  })

  it('keeps a loaded branch when it is collapsed and expanded again', async () => {
    mockGetWorkflowStatus.mockResolvedValue(makeStatus('child-1'))

    const { result } = renderBranch('child-1', 'child')
    act(() => result.current.toggle())
    await waitFor(() => expect(result.current.status).not.toBeNull())

    act(() => result.current.toggle())
    expect(result.current.expanded).toBe(false)
    act(() => result.current.toggle())

    expect(result.current.expanded).toBe(true)
    expect(result.current.loading).toBe(false)
    expect(result.current.status?.workflow_id).toBe('child-1')
    expect(mockGetWorkflowStatus).toHaveBeenCalledTimes(1)
  })

  it('loads a branch again when it is expanded after a failed load', async () => {
    mockGetWorkflowStatus.mockRejectedValueOnce(new Error('500'))
    mockGetWorkflowStatus.mockResolvedValueOnce(makeStatus('child-1'))

    const { result } = renderBranch('child-1', 'child')
    act(() => result.current.toggle())
    await waitFor(() => expect(result.current.error).toBe('loadFailed'))

    act(() => result.current.toggle())
    act(() => result.current.toggle())

    await waitFor(() => expect(result.current.status?.workflow_id).toBe('child-1'))
    expect(result.current.error).toBeNull()
    expect(mockGetWorkflowStatus).toHaveBeenCalledTimes(2)
  })

  it('does not load a missing execution again on re-expand', async () => {
    mockGetWorkflowStatus.mockResolvedValue(null)

    const { result } = renderBranch('child-gone', 'child')
    act(() => result.current.toggle())
    await waitFor(() => expect(result.current.error).toBe('notFound'))

    act(() => result.current.toggle())
    act(() => result.current.toggle())

    expect(result.current.error).toBe('notFound')
    expect(mockGetWorkflowStatus).toHaveBeenCalledTimes(1)
  })

  it('follows "Expand all" and "Collapse all", loading on the first expand', async () => {
    mockGetWorkflowStatus.mockResolvedValue(makeStatus('child-1'))

    const { result, rerender } = renderBranch('child-1', 'child')

    rerender({ signal: { gen: 1, expand: true } })
    expect(result.current.expanded).toBe(true)
    await waitFor(() => expect(result.current.status).not.toBeNull())

    rerender({ signal: { gen: 2, expand: false } })
    expect(result.current.expanded).toBe(false)

    rerender({ signal: { gen: 3, expand: true } })
    expect(result.current.expanded).toBe(true)
    expect(mockGetWorkflowStatus).toHaveBeenCalledTimes(1)
  })

  it('picks up an "Expand all" issued before the branch mounted', async () => {
    mockGetWorkflowStatus.mockResolvedValue(makeStatus('child-1'))

    const { result } = renderBranch('child-1', 'child', { gen: 4, expand: true })

    expect(result.current.expanded).toBe(true)
    await waitFor(() => expect(result.current.status?.workflow_id).toBe('child-1'))
  })

  it('never loads a node with no task workflow, even on "Expand all"', () => {
    const { result } = renderBranch('', 'task', { gen: 1, expand: true })

    expect(result.current.expanded).toBe(true)
    expect(result.current.loading).toBe(false)
    expect(mockGetTaskStatus).not.toHaveBeenCalled()
  })

  it('reloads the branch on refetch', async () => {
    mockGetWorkflowStatus.mockResolvedValueOnce(makeStatus('child-1'))
    mockGetWorkflowStatus.mockResolvedValueOnce({ ...makeStatus('child-1'), status: 'COMPLETED' })

    const { result } = renderBranch('child-1', 'child')
    act(() => result.current.toggle())
    await waitFor(() => expect(result.current.status?.status).toBe('RUNNING'))

    act(() => result.current.refetch())

    await waitFor(() => expect(result.current.status?.status).toBe('COMPLETED'))
    expect(mockGetWorkflowStatus).toHaveBeenCalledTimes(2)
  })
})
