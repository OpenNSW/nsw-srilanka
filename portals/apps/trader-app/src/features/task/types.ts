export type TaskCommand = 'SUBMISSION' | 'SAVE_AS_DRAFT'

export interface TaskCommandRequest {
  command: TaskCommand
  taskId: string
  workflowId: string
  data: Record<string, unknown>
}

export type TaskCommandResponse = {
  success: boolean
  data: Record<string, unknown>
  error?: { code: string; message: string; details: unknown }
}
