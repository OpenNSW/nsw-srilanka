import type {
  Consignment,
  ConsignmentListResult,
  ConsignmentState,
  CreateConsignmentResponse,
  TradeFlow,
  Workflow,
  WorkflowQueryParams,
  WorkflowResponse,
  WorkflowTemplate,
} from './types'
import { http, HttpError } from '@/services/http'
import { API_BASE_URL } from '@/constants'

// Workflow functions
export async function getWorkflowsByHSCode(
  params: WorkflowQueryParams,
  signal?: AbortSignal,
): Promise<WorkflowResponse> {
  const [importWorkflow, exportWorkflow] = await Promise.all([
    fetchWorkflowByType(params.hs_code, 'IMPORT', signal),
    fetchWorkflowByType(params.hs_code, 'EXPORT', signal),
  ])

  return {
    import: importWorkflow ? [importWorkflow] : [],
    export: exportWorkflow ? [exportWorkflow] : [],
  }
}

async function fetchWorkflowByType(
  hsCode: string,
  tradeFlow: 'IMPORT' | 'EXPORT',
  signal?: AbortSignal,
): Promise<Workflow | null> {
  try {
    const { data } = await http.request<WorkflowTemplate>({
      url: `${API_BASE_URL}/api/v1/workflows/templates`,
      params: { hsCode, tradeFlow },
      attachToken: true,
      signal,
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

export async function getWorkflowById(id: string, signal?: AbortSignal): Promise<Workflow | undefined> {
  const { data } = await http.request<Workflow>({
    url: `${API_BASE_URL}/api/v1/workflows/${id}`,
    attachToken: true,
    signal,
  })
  return data
}

// Consignment functions
export async function createConsignment(signal?: AbortSignal): Promise<CreateConsignmentResponse> {
  const { data } = await http.request<CreateConsignmentResponse>({
    url: `${API_BASE_URL}/api/v1/consignments`,
    method: 'POST',
    data: {},
    attachToken: true,
    signal,
  })
  return data
}

// Starts one of the consignment's startableFlows as its own workflow on the consignment; its
// tasks then appear among the consignment's workflow nodes. The request only names the flow:
// the server sets its variables, and the flow's own tasks collect anything from the user.
export async function startConsignmentFlow(
  consignmentId: string,
  templateId: string,
  signal?: AbortSignal,
): Promise<void> {
  await http.request({
    url: `${API_BASE_URL}/api/v1/consignments/${encodeURIComponent(consignmentId)}/flows`,
    method: 'POST',
    data: { templateId },
    attachToken: true,
    signal,
  })
}

export async function getConsignment(id: string, signal?: AbortSignal): Promise<Consignment | null> {
  try {
    const { data } = await http.request<Consignment>({
      url: `${API_BASE_URL}/api/v1/consignments/${id}`,
      attachToken: true,
      signal,
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
  q?: string,
  signal?: AbortSignal,
): Promise<ConsignmentListResult> {
  const params: Record<string, string | number> = { offset, limit }
  if (state && state !== 'all') params.state = state
  if (flow && flow !== 'all') params.flow = flow
  if (q && q.trim() !== '') params.q = q.trim()

  const { data } = await http.request<ConsignmentListResult>({
    url: `${API_BASE_URL}/api/v1/consignments`,
    params,
    attachToken: true,
    signal,
  })
  return data
}
