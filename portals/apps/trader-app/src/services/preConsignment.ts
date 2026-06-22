import { http } from './http'
import { API_BASE_URL } from '../constants'
import { sendTaskCommand } from '@/features/task/service'

export type PreConsignmentState = 'LOCKED' | 'READY' | 'IN_PROGRESS' | 'COMPLETED'
export type WorkflowNodeState = 'LOCKED' | 'READY' | 'IN_PROGRESS' | 'COMPLETED' | 'FAILED'

export interface WorkflowNodeTemplate {
  name: string
  description: string
  type: string
}

export interface WorkflowNode {
  id: string
  state: WorkflowNodeState
  workflowNodeTemplate: WorkflowNodeTemplate
  createdAt: string
  updatedAt: string
  depends_on?: string[]
}

export interface PreConsignmentTemplate {
  id: string
  name: string
  description: string
  dependsOn: string[]
}

export interface PreConsignmentInstance {
  id: string
  traderId: string
  state: PreConsignmentState
  traderContext: Record<string, unknown>
  createdAt: string
  updatedAt: string
  preConsignmentTemplate: PreConsignmentTemplate
  workflowNodes: WorkflowNode[]
}

export interface TraderPreConsignmentItem {
  id: string
  name: string
  description: string
  state: PreConsignmentState
  dependsOn: string[]
  preConsignment?: PreConsignmentInstance
  preConsignmentTemplate?: PreConsignmentTemplate
}

import type { PaginatedResponse } from './types/common'

export type TraderPreConsignmentsResponse = PaginatedResponse<TraderPreConsignmentItem>

type PreConsignmentListApiResponse = PreConsignmentInstance[] | TraderPreConsignmentsResponse

export interface CreatePreConsignmentRequest {
  preConsignmentTemplateId: string
}

export interface TaskCommandRequest {
  command: 'SUBMISSION' | 'SAVE_AS_DRAFT'
  taskId: string
  workflowId: string
  data?: Record<string, unknown>
}

export interface TaskCommandResponse {
  success: boolean
  message?: string
  data?: unknown
}

export async function getTraderPreConsignments(
  offset: number = 0,
  limit: number = 50,
): Promise<TraderPreConsignmentsResponse> {
  const { data } = await http.request<PreConsignmentListApiResponse>({
    url: `${API_BASE_URL}/api/v1/pre-consignments`,
    params: { offset, limit },
    attachToken: true,
  })

  if (Array.isArray(data)) {
    const items: TraderPreConsignmentItem[] = data.map((instance) => ({
      id: instance.preConsignmentTemplate.id,
      name: instance.preConsignmentTemplate.name,
      description: instance.preConsignmentTemplate.description,
      state: instance.state,
      dependsOn: instance.preConsignmentTemplate.dependsOn,
      preConsignment: instance,
      preConsignmentTemplate: instance.preConsignmentTemplate,
    }))

    return {
      total: items.length,
      items,
      offset: 0,
      limit: items.length,
    }
  }
  return data
}

export async function getPreConsignment(id: string): Promise<PreConsignmentInstance> {
  const { data } = await http.request<PreConsignmentInstance>({
    url: `${API_BASE_URL}/api/v1/pre-consignments/${id}`,
    attachToken: true,
  })
  return data
}

export async function createPreConsignment(templateId: string): Promise<PreConsignmentInstance> {
  const { data } = await http.request<PreConsignmentInstance>({
    url: `${API_BASE_URL}/api/v1/pre-consignments`,
    method: 'POST',
    data: { preConsignmentTemplateId: templateId } satisfies CreatePreConsignmentRequest,
    attachToken: true,
  })
  return data
}

export async function submitPreConsignmentTask(request: TaskCommandRequest): Promise<TaskCommandResponse> {
  return sendTaskCommand({
    command: request.command === 'SAVE_AS_DRAFT' ? 'SAVE_AS_DRAFT' : 'SUBMISSION',
    taskId: request.taskId,
    workflowId: request.workflowId,
    data: request.data || {},
  })
}
