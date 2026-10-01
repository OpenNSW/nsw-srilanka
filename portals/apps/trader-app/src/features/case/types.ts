import type { PaginatedResponse } from '@/services/types/common'
import type { ConsignmentState } from '@/features/consignment/types'

// One row of GET /api/v1/cases.
export interface CaseSummary {
  id: string
  name?: string
  state: ConsignmentState
  createdAt: string
  updatedAt: string
}

export type CaseListResult = PaginatedResponse<CaseSummary>
