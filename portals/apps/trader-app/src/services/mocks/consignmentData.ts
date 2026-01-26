export interface Consignment {
  id: string
  workflowId: string
  createdAt: string
  status: 'in_progress' | 'completed'
}

// Mock consignments - maps consignment IDs to their workflow IDs
export const mockConsignments: Consignment[] = [
  // Green tea imports
  { id: 'CON-001', workflowId: 'wf-09021011-import', createdAt: '2024-01-15', status: 'in_progress' },
  { id: 'CON-002', workflowId: 'wf-09021012-import', createdAt: '2024-01-16', status: 'in_progress' },
  { id: 'CON-003', workflowId: 'wf-09021013-import', createdAt: '2024-01-17', status: 'in_progress' },

  // Green tea exports
  { id: 'CON-004', workflowId: 'wf-09021011-export', createdAt: '2024-01-18', status: 'in_progress' },
  { id: 'CON-005', workflowId: 'wf-09021012-export', createdAt: '2024-01-19', status: 'in_progress' },

  // Black tea imports
  { id: 'CON-006', workflowId: 'wf-09023011-import', createdAt: '2024-01-20', status: 'in_progress' },
  { id: 'CON-007', workflowId: 'wf-09023012-import', createdAt: '2024-01-21', status: 'in_progress' },

  // Black tea exports
  { id: 'CON-008', workflowId: 'wf-09023011-export', createdAt: '2024-01-22', status: 'in_progress' },
  { id: 'CON-009', workflowId: 'wf-09023012-export', createdAt: '2024-01-23', status: 'in_progress' },
]

export function getConsignmentById(consignmentId: string): Consignment | undefined {
  return mockConsignments.find((c) => c.id === consignmentId)
}