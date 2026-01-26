import { apiGet, apiPost, USE_MOCK } from './api'
import { findTaskDetails, type TaskDetails } from './mocks/taskData'

export type TaskCommand = 'SUBMISSION' | 'DRAFT'

export interface TaskCommandRequest {
  command: TaskCommand
  taskId: string
  consignmentId: string
  data: Record<string, unknown>
}

export interface TaskCommandResponse {
  success: boolean
  message: string
  taskId: string
  status?: string
}

export async function getTaskDetails(
  consignmentId: string,
  taskId: string
): Promise<TaskDetails> {
  console.log(
    `Fetching task details for consignment: ${consignmentId}, task: ${taskId}`
  )

  if (USE_MOCK) {
    // Simulate network delay
    await new Promise((resolve) => setTimeout(resolve, 300))

    // Find the task details based on taskId
    const taskDetails = findTaskDetails(taskId)
    if (!taskDetails) {
      throw new Error(`Task not found: ${taskId}`)
    }

    return taskDetails
  }

  return apiGet<TaskDetails>(`/workflows/${consignmentId}/tasks/${taskId}`)
}

export async function sendTaskCommand(
  request: TaskCommandRequest
): Promise<TaskCommandResponse> {
  console.log(`Sending ${request.command} command for task: ${request.taskId}`, request)

  if (USE_MOCK) {
    // Simulate network delay
    await new Promise((resolve) => setTimeout(resolve, 500))

    // Mock successful response
    return {
      success: true,
      message: request.command === 'DRAFT'
        ? 'Draft saved successfully'
        : 'Task submitted successfully',
      taskId: request.taskId,
      status: request.command === 'DRAFT' ? 'DRAFT' : 'SUBMITTED',
    }
  }

  return apiPost<TaskCommandRequest, TaskCommandResponse>(
    `/tasks/${request.taskId}/command`,
    request
  )
}