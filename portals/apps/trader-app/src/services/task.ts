import { apiGet, apiPost, type ApiResponse } from './api'
import type { RenderInfo } from "../plugins";

export type TaskAction = 'FETCH_FORM' | 'SUBMIT_FORM' | 'SAVE_AS_DRAFT'

export type TaskCommand = 'SUBMISSION' | 'SAVE_AS_DRAFT'

export interface TaskFormData {
  title: string
  schema: any
  uiSchema?: any
  formData: any
}

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
    action: TaskAction
    content: Record<string, unknown>
  }
}

const TASKS_API_URL = '/tasks'

export async function getTaskInfo(taskId: string): Promise<RenderInfo> {
  const response = await apiGet<{ success: boolean; data: RenderInfo }>(`${TASKS_API_URL}/${taskId}`)
  if (!response.data) {
    throw new Error('Failed to fetch task information')
  }
  return response.data
}

export async function sendTaskCommand(
  request: TaskCommandRequest
): Promise<TaskCommandResponse> {
  console.log(`Sending ${request.command} command for task: ${request.taskId}`, request)

  // Use POST /api/tasks with action type and submission data
  const action: TaskAction = request.command === 'SAVE_AS_DRAFT' ? 'SAVE_AS_DRAFT' : 'SUBMIT_FORM'

  return apiPost<SendTaskCommandRequest, TaskCommandResponse>(TASKS_API_URL, {
    task_id: request.taskId,
    workflow_id: request.workflowId,
    payload: {
      action,
      content: request.data,
    },
  })
}