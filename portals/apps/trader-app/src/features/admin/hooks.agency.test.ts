import { renderHook, waitFor } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { useAdminConsignment } from '@/features/admin/hooks'
import { getCaseForAdmin, getConsignmentForAdmin } from '@/features/admin/service'
import type { AdminConsignmentDetail } from '@/features/admin/types'

// An agency deployment shows cases. isAgencyMode is read once as the hooks module loads, so this
// case gets a file of its own.
vi.mock('@/features/case/mode', () => ({ isAgencyMode: true }))
vi.mock('@/features/admin/service', () => ({
  getConsignmentForAdmin: vi.fn(),
  getCaseForAdmin: vi.fn(),
  getWorkflowEngineStatus: vi.fn(),
  getTaskWorkflowEngineStatus: vi.fn(),
  forceParkNode: vi.fn(),
  resolveAdminIntervention: vi.fn(),
}))

describe('useAdminConsignment in an agency', () => {
  it('loads the case through the admin cases endpoint', async () => {
    vi.mocked(getCaseForAdmin).mockResolvedValue({ id: 'case-1', flows: [] } as unknown as AdminConsignmentDetail)

    const { result } = renderHook(() => useAdminConsignment('case-1'))

    await waitFor(() => expect(result.current.consignment?.id).toBe('case-1'))
    expect(getCaseForAdmin).toHaveBeenCalledWith('case-1', expect.any(AbortSignal))
    expect(getConsignmentForAdmin).not.toHaveBeenCalled()
  })
})
