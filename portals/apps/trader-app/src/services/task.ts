import {apiPost} from './api'
import type { StepType } from './types/consignment'
import type {JsonSchema, UISchemaElement} from "../components/JsonForm";

export type TaskAction = 'FETCH_FORM' | 'SUBMIT_FORM' | 'DRAFT'

export interface TaskFormData {
  title: string
  schema: JsonSchema
  uiSchema: UISchemaElement
  formData: Record<string, unknown>
}

export interface ExecuteTaskResponse {
  success: boolean
  data: TaskFormData
}

export interface ExecuteTaskRequest {
  task_id: string
  consignment_id: string
  payload: {
    action: TaskAction
  }
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

export interface SendTaskCommandRequest {
  task_id: string
  consignment_id: string
  payload: {
    action: TaskAction
    content: Record<string, unknown>
  }
}

const TASKS_API_URL = '/tasks'

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

  return apiPost<ExecuteTaskRequest, ExecuteTaskResponse>(TASKS_API_URL, {
    task_id: taskId,
    consignment_id: consignmentId,
    payload: {
      action,
    },
  })
}

export async function sendTaskCommand(
  request: TaskCommandRequest
): Promise<TaskCommandResponse> {
  console.log(`Sending ${request.command} command for task: ${request.taskId}`, request)

  // Use POST /api/tasks with action type and submission data
  const action: TaskAction = request.command === 'DRAFT' ? 'DRAFT' : 'SUBMIT_FORM'

  return apiPost<SendTaskCommandRequest, TaskCommandResponse>(TASKS_API_URL, {
    task_id: request.taskId,
    consignment_id: request.consignmentId,
    payload: {
      action,
      content: request.data,
    },
  })
}