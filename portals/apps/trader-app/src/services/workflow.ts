import { apiGet } from './api'
import type { Workflow, WorkflowTemplate, WorkflowQueryParams } from './types/workflow'

export interface WorkflowResponse {
  import: Workflow[]
  export: Workflow[]
}

const WORKFLOW_TEMPLATES_ENDPOINT = '/workflows/templates'

export async function getWorkflowsByHSCode(
  params: WorkflowQueryParams
): Promise<WorkflowResponse> {

  // Fetch import and export workflows in parallel
  const [importWorkflow, exportWorkflow] = await Promise.all([
    fetchWorkflowByType(params.hs_code, 'IMPORT'),
    fetchWorkflowByType(params.hs_code, 'EXPORT'),
  ])

  return {
    import: importWorkflow ? [importWorkflow] : [],
    export: exportWorkflow ? [exportWorkflow] : [],
  }
}

async function fetchWorkflowByType(
  hsCode: string,
  tradeFlow: 'IMPORT' | 'EXPORT'
): Promise<Workflow | null> {
  try {
    const template = await apiGet<WorkflowTemplate>(WORKFLOW_TEMPLATES_ENDPOINT, {
      hsCode,
      tradeFlow,
    })

    // Transform WorkflowTemplate to Workflow
    return {
      id: template.id,
      name: template.version,
      type: tradeFlow.toLowerCase() as 'import' | 'export',
      steps: template.steps,
    }
  } catch (error) {
    // Return null for 404 errors (workflow not found)
    if (error instanceof Error && error.message.includes('404')) {
      return null
    }
    throw error
  }
}