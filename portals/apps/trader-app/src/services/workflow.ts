import { http } from './http'
import { API_BASE_URL, API_PATH_PREFIX } from '../constants'
import type { Workflow, WorkflowTemplate, WorkflowQueryParams } from './types/workflow'

export interface WorkflowResponse {
  import: Workflow[]
  export: Workflow[]
}

const BASE = `${API_BASE_URL}${API_PATH_PREFIX}`
const WORKFLOW_TEMPLATES_URL = `${BASE}/workflows/templates`

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
    const { data } = await http.request({
      url: WORKFLOW_TEMPLATES_URL,
      params: { hsCode, tradeFlow },
      attachToken: true,
    })

    const template = data as WorkflowTemplate
    return {
      id: template.id,
      name: template.version,
      type: tradeFlow.toLowerCase() as 'import' | 'export',
      steps: template.steps,
    }
  } catch (error) {
    if (error instanceof Error && error.message.includes('404')) {
      return null
    }
    throw error
  }
}

export async function getWorkflowById(id: string): Promise<Workflow | undefined> {
  const { data } = await http.request({
    url: `${BASE}/workflows/${id}`,
    attachToken: true,
  })
  return data as Workflow
}
