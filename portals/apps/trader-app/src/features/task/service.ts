import { http } from '@/services/http'
import { API_BASE_URL } from '@/constants'
import type { ZoneView } from '@/features/zone/types'
import type { TaskCommandRequest, TaskCommandResponse } from './types'

export async function getZoneView(taskId: string): Promise<ZoneView> {
  const { data } = await http.request<ZoneView>({
    url: `${API_BASE_URL}/api/v1/tasks/${taskId}`,
    attachToken: true,
  })
  return data
}

export async function submitTaskStep(
  taskId: string,
  stepId: string,
  command: string,
  payload: Record<string, unknown>,
): Promise<void> {
  await http.request({
    url: `${API_BASE_URL}/api/v1/tasks/${encodeURIComponent(taskId)}/steps/${encodeURIComponent(stepId)}`,
    method: 'POST',
    data: { command, payload },
    attachToken: true,
  })
}

// claimTask claims the task for the signed-in user: POST /claim (204). A 409 means
// another user holds it or the task is completed; refetch the task either way.
export async function claimTask(taskId: string): Promise<void> {
  await http.request({
    url: `${API_BASE_URL}/api/v1/tasks/${encodeURIComponent(taskId)}/claim`,
    method: 'POST',
    attachToken: true,
  })
}

// releaseTask releases the signed-in user's claim: POST /release (204). A 403 means
// another user holds it.
export async function releaseTask(taskId: string): Promise<void> {
  await http.request({
    url: `${API_BASE_URL}/api/v1/tasks/${encodeURIComponent(taskId)}/release`,
    method: 'POST',
    attachToken: true,
  })
}

export async function sendTaskCommand(request: TaskCommandRequest): Promise<TaskCommandResponse> {
  const action: string = request.command === 'SAVE_AS_DRAFT' ? 'SAVE_AS_DRAFT' : 'SUBMIT_FORM'

  const { data } = await http.request<TaskCommandResponse>({
    url: `${API_BASE_URL}/api/v1/tasks/${request.taskId}`,
    method: 'POST',
    data: {
      command: action,
      payload: {
        workflow_id: request.workflowId,
        content: request.data,
      },
    },
    attachToken: true,
  })
  return data
}
