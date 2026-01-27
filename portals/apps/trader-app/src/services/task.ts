import { apiGet } from './api'
import type { StepType } from './types/consignment'
import type {JsonSchema, UISchemaElement} from "../components/JsonForm";
import type {TaskDetails} from "./types/taskData.ts";

export type TaskAction = 'FETCH_FORM' | 'SUBMIT_FORM' | 'DRAFT'

export interface TaskFormData {
  title: string
  schema: JsonSchema
  uiSchema: UISchemaElement
  formData: Record<string, unknown>
}

export interface ExecuteTaskResult {
  status: string
  message: string
  data: TaskFormData
}

export interface ExecuteTaskResponse {
  success: boolean
  result: ExecuteTaskResult
}

export type TaskCommand = 'SUBMISSION' | 'DRAFT'

export interface TaskCommandRequest {
  command: TaskCommand
  taskId: string
  consignmentId: string
  data: Record<string, unknown>
}

export interface TaskCommandResponse {
  success: boolean
  message: string
  taskId: string
  status?: string
}

const TASKS_API_URL = 'http://localhost:8080/api/tasks'

function getActionForStepType(stepType: StepType): TaskAction {
  switch (stepType) {
    case 'SIMPLE_FORM':
      return 'FETCH_FORM'
    default:
      return 'FETCH_FORM'
  }
}

export async function executeTask(
  consignmentId: string,
  taskId: string,
  stepType: StepType
): Promise<ExecuteTaskResponse> {
  const action = getActionForStepType(stepType)

  const response = await fetch(TASKS_API_URL, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
    },
    body: JSON.stringify({
      task_id: taskId,
      consignment_id: consignmentId,
      payload: {
        action,
      },
    }),
  })

  if (!response.ok) {
    throw new Error(`API error: ${response.status} ${response.statusText}`)
  }

  return response.json()
}

export async function getTaskDetails(
  consignmentId: string,
  taskId: string
): Promise<TaskDetails> {
  console.log(
    `Fetching task details for consignment: ${consignmentId}, task: ${taskId}`
  )

  return apiGet<TaskDetails>(`/workflows/${consignmentId}/tasks/${taskId}`)
}

export async function sendTaskCommand(
  request: TaskCommandRequest
): Promise<TaskCommandResponse> {
  console.log(`Sending ${request.command} command for task: ${request.taskId}`, request)

  // Use POST /api/tasks with action type and submission data
  const action: TaskAction = request.command === 'DRAFT' ? 'DRAFT' : 'SUBMIT_FORM'

  const response = await fetch(TASKS_API_URL, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
    },
    body: JSON.stringify({
      task_id: request.taskId,
      consignment_id: request.consignmentId,
      payload: {
        action,
        content: request.data,
      },
    }),
  })

  if (!response.ok) {
    throw new Error(`API error: ${response.status} ${response.statusText}`)
  }

  return response.json()
}