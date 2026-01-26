import { apiGet, apiPost, USE_MOCK } from './api'
import type { Workflow, WorkflowQueryParams } from './types/workflow'
import { mockWorkflows } from './mocks/workflowData'
import { findTaskDetails, type TaskDetails } from './mocks/taskData'

export interface WorkflowResponse {
  import: Workflow[]
  export: Workflow[]
}

function getMockWorkflows(params: WorkflowQueryParams): WorkflowResponse {
  const { hs_code } = params

  // Find workflows where the workflow's hsCode starts with the searched code
  // This allows searching for parent codes to find child workflows
  // e.g., searching "0902" returns workflows for "090210", "090220", etc.
  const workflows = mockWorkflows.filter((wf) => wf.hsCode.startsWith(hs_code))

  return {
    import: workflows.filter((wf) => wf.type === 'import'),
    export: workflows.filter((wf) => wf.type === 'export'),
  }
}

export async function getWorkflowsByHSCode(
  params: WorkflowQueryParams
): Promise<WorkflowResponse> {
  if (USE_MOCK) {
    // Simulate network delay
    await new Promise((resolve) => setTimeout(resolve, 200))
    return getMockWorkflows(params)
  }

  return apiGet<WorkflowResponse>('/workflows', {
    hs_code: params.hs_code,
  })
}

export async function getWorkflowById(id: string): Promise<Workflow | undefined> {
  if (USE_MOCK) {
    // Simulate network delay
    await new Promise((resolve) => setTimeout(resolve, 200))
    return mockWorkflows.find((wf) => wf.id === id)
  }

  return apiGet<Workflow>(`/workflows/${id}`)
}

export async function executeTask(
  consignmentId: string,
  taskId: string
): Promise<TaskDetails> {
  if (USE_MOCK) {
    // Simulate network delay
    await new Promise((resolve) => setTimeout(resolve, 500))

    // Find the task details based on taskId
    const taskDetails = findTaskDetails(taskId)
    if (!taskDetails) {
      throw new Error(`Task not found: ${taskId}`)
    }

    return taskDetails
  }
  return apiPost<Record<string, never>, TaskDetails>(`/workflows/${consignmentId}/tasks/${taskId}`, {})
}