import { http, HttpError } from './http'
import { API_BASE_URL } from '../constants'
import type { Workflow, WorkflowTemplate, WorkflowQueryParams } from './types/workflow'

export interface WorkflowResponse {
  import: Workflow[]
  export: Workflow[]
}

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
