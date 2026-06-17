import { defaultApiClient, type ApiClient, type ApiResponse, apiPost } from './api'
import type { ZoneView } from '../zones/types'

export type TaskCommand = 'SUBMISSION' | 'SAVE_AS_DRAFT'

export interface TaskCommandRequest {
  command: TaskCommand
  taskId: string
  workflowId: string
  data: Record<string, unknown>
}

export type TaskCommandResponse = ApiResponse<Record<string, unknown>>

export interface SendTaskCommandRequest {
  task_id: string
  workflow_id: string
  payload: {
    action: string
    content: Record<string, unknown>
  }
}

const TASKS_API_URL = '/tasks'

export async function getZoneView(taskId: string, apiClient: ApiClient = defaultApiClient): Promise<ZoneView> {
  return apiClient.get<ZoneView>(`${TASKS_API_URL}/${taskId}`)
}

export async function submitTaskStep(
  taskId: string,
  command: string,
  payload: Record<string, unknown>,
  apiClient: ApiClient = defaultApiClient,
): Promise<void> {
  await apiClient.post<Record<string, unknown>, unknown>(`${TASKS_API_URL}/${taskId}/command/${command}`, payload)
}

export async function sendTaskAction(taskId: string, workflowId: string, action: string): Promise<TaskCommandResponse> {
  return apiPost<SendTaskCommandRequest, TaskCommandResponse>(TASKS_API_URL, {
    task_id: taskId,
    workflow_id: workflowId,
    payload: { action, content: {} },
  })
}

export async function sendTaskCommand(
  request: TaskCommandRequest,
  apiClient: ApiClient = defaultApiClient,
): Promise<TaskCommandResponse> {
  console.log(`Sending ${request.command} command for task: ${request.taskId}`, request)

  // Use POST /api/tasks with action type and submission data
  const action: string = request.command === 'SAVE_AS_DRAFT' ? 'SAVE_AS_DRAFT' : 'SUBMIT_FORM'

  return apiClient.post<SendTaskCommandRequest, TaskCommandResponse>(TASKS_API_URL, {
    task_id: request.taskId,
    workflow_id: request.workflowId,
    payload: {
      action,
      content: request.data,
    },
  })
}
