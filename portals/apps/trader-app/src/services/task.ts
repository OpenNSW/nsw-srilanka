import {apiGet, apiPost, type ApiResponse} from './api'
import type {RenderInfo} from "../plugins";

export type TaskAction = 'FETCH_FORM' | 'SUBMIT_FORM' | 'DRAFT'

export type TaskCommand = 'SUBMISSION' | 'DRAFT'

export interface TaskCommandRequest {
  command: TaskCommand
  taskId: string
  workflowId: string
  data: Record<string, unknown>
}

export interface TaskCommandResponse {
  success: boolean
  message: string
  taskId: string
  status?: string
}

export interface SendTaskCommandRequest {
  task_id: string
  workflow_id: string
  payload: {
    action: TaskAction
    content: Record<string, unknown>
  }
}

const TASKS_API_URL = '/tasks'

export async function getTaskInfo(taskId: string): Promise<ApiResponse<RenderInfo>> {
  return apiGet<ApiResponse<RenderInfo>>(`${TASKS_API_URL}/${taskId}`)
}

export async function sendTaskCommand(
  request: TaskCommandRequest
): Promise<TaskCommandResponse> {
  console.log(`Sending ${request.command} command for task: ${request.taskId}`, request)

  // Use POST /api/tasks with action type and submission data
  const action: TaskAction = request.command === 'DRAFT' ? 'DRAFT' : 'SUBMIT_FORM'

  return apiPost<SendTaskCommandRequest, TaskCommandResponse>(TASKS_API_URL, {
    task_id: request.taskId,
    workflow_id: request.workflowId,
    payload: {
      action,
      content: request.data,
    },
  })
}