import { http } from './http'
import { API_BASE_URL, API_PATH_PREFIX } from '../constants'
import type { ZoneView } from '../zones/types'

export type TaskCommand = 'SUBMISSION' | 'SAVE_AS_DRAFT'

export interface TaskCommandRequest {
  command: TaskCommand
  taskId: string
  workflowId: string
  data: Record<string, unknown>
}

export type TaskCommandResponse = {
  success: boolean
  data: Record<string, unknown>
  error?: { code: string; message: string; details: unknown }
}

export interface SendTaskCommandRequest {
  task_id: string
  workflow_id: string
  payload: {
    action: string
    content: Record<string, unknown>
  }
}

const BASE = `${API_BASE_URL}${API_PATH_PREFIX}`
const TASKS_URL = `${BASE}/tasks`

export async function getZoneView(taskId: string): Promise<ZoneView> {
  const { data } = await http.request({
    url: `${TASKS_URL}/${taskId}`,
    attachToken: true,
  })
  return data as ZoneView
}

export async function submitTaskStep(taskId: string, payload: Record<string, unknown>): Promise<void> {
  await http.request({
    url: `${TASKS_URL}/${taskId}`,
    method: 'POST',
    data: payload,
    attachToken: true,
  })
}

export async function sendTaskCommand(request: TaskCommandRequest): Promise<TaskCommandResponse> {
  console.log(`Sending ${request.command} command for task: ${request.taskId}`, request)

  const action: string = request.command === 'SAVE_AS_DRAFT' ? 'SAVE_AS_DRAFT' : 'SUBMIT_FORM'

  const { data } = await http.request({
    url: TASKS_URL,
    method: 'POST',
    data: {
      task_id: request.taskId,
      workflow_id: request.workflowId,
      payload: {
        action,
        content: request.data,
      },
    } satisfies SendTaskCommandRequest,
    attachToken: true,
  })
  return data as TaskCommandResponse
}
