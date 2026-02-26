// portals/apps/trader-app/src/services/preConsignment.ts

import { apiGet, apiPost } from './api'
import type { TaskFormData } from './task'
import { sendTaskCommand, getTaskInfo } from './task'

// --- Types based on Backend DTOs ---

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
    workflowNodeTemplate: WorkflowNodeTemplate // Nested object
    createdAt: string
    updatedAt: string
    // Note: Backend JSON response showed snake_case 'depends_on' for nodes
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
    id: string // Template ID
    name: string
    description: string
    state: PreConsignmentState
    dependsOn: string[] // From template
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

// --- API Methods ---

export async function getTraderPreConsignments(
    offset: number = 0,
    limit: number = 50
): Promise<TraderPreConsignmentsResponse> {
    const response = await apiGet<PreConsignmentListApiResponse>('/pre-consignments', { offset, limit })

    if (Array.isArray(response)) {
        const items: TraderPreConsignmentItem[] = response.map((instance) => ({
            id: instance.preConsignmentTemplate.id,
            name: instance.preConsignmentTemplate.name,
            description: instance.preConsignmentTemplate.description,
            state: instance.state,
            dependsOn: instance.preConsignmentTemplate.dependsOn,
            preConsignment: instance,
            preConsignmentTemplate: instance.preConsignmentTemplate,
        }))

        return {
            totalCount: items.length,
            items,
            offset: 0,
            limit: items.length,
        }
    }
    return response
}

export async function getPreConsignment(
    id: string
): Promise<PreConsignmentInstance> {
    return apiGet<PreConsignmentInstance>(`/pre-consignments/${id}`)
}

export async function createPreConsignment(
    templateId: string
): Promise<PreConsignmentInstance> {
    const payload: CreatePreConsignmentRequest = {
        preConsignmentTemplateId: templateId,
    }
    return apiPost<CreatePreConsignmentRequest, PreConsignmentInstance>(
        '/pre-consignments',
        payload
    )
}

// Fetch the form schema (GET endpoint)
export async function fetchPreConsignmentTaskForm(
    taskId: string
): Promise<TaskFormData> {
    const renderInfo = await getTaskInfo(taskId)
    // Extract form data from the render info structure
    // For SIMPLE_FORM type, content has traderFormInfo
    if (renderInfo.type === 'SIMPLE_FORM' && 'traderFormInfo' in renderInfo.content) {
        const traderFormInfo = renderInfo.content.traderFormInfo
        return {
            title: traderFormInfo.title,
            schema: traderFormInfo.schema,
            uiSchema: traderFormInfo.uiSchema,
            formData: traderFormInfo.formData || {}
        }
    }
    throw new Error('Unexpected task response structure')
}

// Submit the form data (Action: SUBMIT_FORM or DRAFT)
export async function submitPreConsignmentTask(
    request: TaskCommandRequest
): Promise<TaskCommandResponse> {
    return sendTaskCommand({
        command: request.command === 'SAVE_AS_DRAFT' ? 'SAVE_AS_DRAFT' : 'SUBMISSION',
        taskId: request.taskId,
        workflowId: request.workflowId,
        data: request.data || {}
    })
}