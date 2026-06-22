import { http } from '@/services/http'
import { API_BASE_URL } from '@/constants'
import type { ZoneView } from '@/zones/types'
import type { TaskCommandRequest, TaskCommandResponse } from './types'

export async function getZoneView(taskId: string): Promise<ZoneView> {
  const { data } = await http.request<ZoneView>({
    url: `${API_BASE_URL}/api/v1/tasks/${taskId}`,
    attachToken: true,
  })
  return data
}

export async function submitTaskStep(taskId: string, command: string, payload: Record<string, unknown>): Promise<void> {
  await http.request({
    url: `${API_BASE_URL}/api/v1/tasks/${taskId}/commands/${command}`,
    method: 'POST',
    data: payload,
    attachToken: true,
  })
}

export async function sendTaskCommand(request: TaskCommandRequest): Promise<TaskCommandResponse> {
  const action: string = request.command === 'SAVE_AS_DRAFT' ? 'SAVE_AS_DRAFT' : 'SUBMIT_FORM'

  const { data } = await http.request<TaskCommandResponse>({
    url: `${API_BASE_URL}/api/v1/tasks/${request.taskId}/commands/${action}`,
    method: 'POST',
    data: {
      workflow_id: request.workflowId,
      content: request.data,
    },
    attachToken: true,
  })
  return data
}
