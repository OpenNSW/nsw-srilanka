import type {
  Consignment,
  ConsignmentListResult,
  ConsignmentState,
  CreateConsignmentResponse,
  CreatePreConsignmentRequest,
  PreConsignmentInstance,
  PreConsignmentListApiResponse,
  TradeFlow,
  TraderPreConsignmentItem,
  TraderPreConsignmentsResponse,
  Workflow,
  WorkflowQueryParams,
  WorkflowResponse,
  WorkflowTemplate,
} from './types'
import { http, HttpError } from '@/services/http'
import { API_BASE_URL } from '@/constants'
import { sendTaskCommand } from '@/features/task/service'
import type { TaskCommandRequest, TaskCommandResponse } from '@/features/task/types'

// Workflow functions
export async function getWorkflowsByHSCode(params: WorkflowQueryParams): Promise<WorkflowResponse> {
  const [importWorkflow, exportWorkflow] = await Promise.all([
    fetchWorkflowByType(params.hs_code, 'IMPORT'),
    fetchWorkflowByType(params.hs_code, 'EXPORT'),
  ])

  return {
    import: importWorkflow ? [importWorkflow] : [],
    export: exportWorkflow ? [exportWorkflow] : [],
  }
}

async function fetchWorkflowByType(hsCode: string, tradeFlow: 'IMPORT' | 'EXPORT'): Promise<Workflow | null> {
  try {
    const { data } = await http.request<WorkflowTemplate>({
      url: `${API_BASE_URL}/api/v1/workflows/templates`,
      params: { hsCode, tradeFlow },
      attachToken: true,
    })

    return {
      id: data.id,
      name: data.version,
      type: tradeFlow.toLowerCase() as 'import' | 'export',
      steps: data.steps,
    }
  } catch (error) {
    if (error instanceof HttpError && error.status === 404) {
      return null
    }
    throw error
  }
}

export async function getWorkflowById(id: string): Promise<Workflow | undefined> {
  const { data } = await http.request<Workflow>({
    url: `${API_BASE_URL}/api/v1/workflows/${id}`,
    attachToken: true,
  })
  return data
}

// Consignment functions
export async function createConsignment(): Promise<CreateConsignmentResponse> {
  const { data } = await http.request<CreateConsignmentResponse>({
    url: `${API_BASE_URL}/api/v1/consignments`,
    method: 'POST',
    data: {},
    attachToken: true,
  })
  return data
}

export async function getConsignment(id: string): Promise<Consignment | null> {
  try {
    const { data } = await http.request<Consignment>({
      url: `${API_BASE_URL}/api/v1/consignments/${id}`,
      attachToken: true,
    })
    return data
  } catch (error) {
    if (error instanceof HttpError && error.status === 404) {
      return null
    }
    throw error
  }
}

export async function getAllConsignments(
  offset: number = 0,
  limit: number = 50,
  state?: ConsignmentState | 'all',
  flow?: TradeFlow | 'all',
  role: 'trader' | 'cha' = 'trader',
  q?: string,
): Promise<ConsignmentListResult> {
  const params: Record<string, string | number> = { offset, limit }
  if (state && state !== 'all') params.state = state
  if (flow && flow !== 'all') params.flow = flow
  params.role = role
  if (q && q.trim() !== '') params.q = q.trim()

  const { data } = await http.request<ConsignmentListResult>({
    url: `${API_BASE_URL}/api/v1/consignments`,
    params,
    attachToken: true,
  })
  return data
}

// PreConsignment functions
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
