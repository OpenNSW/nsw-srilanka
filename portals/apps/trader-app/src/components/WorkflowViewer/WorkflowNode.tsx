import { useState } from 'react'
import { Handle, Position } from '@xyflow/react'
import type { Node, NodeProps } from '@xyflow/react'
import { Text } from '@radix-ui/themes'
import { useParams, useNavigate } from 'react-router-dom'
import type { ConsignmentStep, StepType, StepStatus } from '../../services/types/consignment'
import { executeTask } from '../../services/task'
import {
  FileTextIcon,
  ClockIcon,
  CheckCircledIcon,
  LockClosedIcon,
  PlayIcon,
  UpdateIcon,
} from '@radix-ui/react-icons'

export interface WorkflowNodeData extends Record<string, unknown> {
  step: ConsignmentStep
}

export type WorkflowNodeType = Node<WorkflowNodeData, 'workflowStep'>

const stepTypeConfig: Record<
  StepType,
  { label: string; icon: React.ReactNode }
> = {
  SIMPLE_FORM: {
    label: 'FORM',
    icon: <FileTextIcon className="w-4 h-4" />,
  },
  WAIT_FOR_EVENT: {
    label: 'Waiting',
    icon: <ClockIcon className="w-4 h-4" />,
  },
}

const statusConfig: Record<
  StepStatus,
  {
    bgColor: string
    borderColor: string
    textColor: string
    iconColor: string
    statusIcon?: React.ReactNode
  }
> = {
  COMPLETED: {
    bgColor: 'bg-emerald-50',
    borderColor: 'border-emerald-400',
    textColor: 'text-emerald-700',
    iconColor: 'text-emerald-600',
    statusIcon: <CheckCircledIcon className="w-4 h-4 text-emerald-600" />,
  },
  READY: {
    bgColor: 'bg-blue-50',
    borderColor: 'border-blue-400',
    textColor: 'text-blue-700',
    iconColor: 'text-blue-600',
  },
  IN_PROGRESS: {
    bgColor: 'bg-orange-50',
    borderColor: 'border-orange-400',
    textColor: 'text-orange-700',
    iconColor: 'text-orange-600',
  },
  LOCKED: {
    bgColor: 'bg-slate-100',
    borderColor: 'border-slate-300',
    textColor: 'text-slate-500',
    iconColor: 'text-slate-400',
    statusIcon: <LockClosedIcon className="w-3 h-3 text-slate-400" />,
  },
  REJECTED: {
    bgColor: 'bg-red-50',
    borderColor: 'border-red-400',
    textColor: 'text-red-700',
    iconColor: 'text-red-600',
  },
}

export function WorkflowNode({ data }: NodeProps<WorkflowNodeType>) {
  const { step } = data
  const { consignmentId } = useParams<{ consignmentId: string }>()
  const navigate = useNavigate()
  const [isLoading, setIsLoading] = useState(false)

  const typeConfig = stepTypeConfig[step.type]
  const statusStyle = statusConfig[step.status]

  const isExecutable = step.status === 'READY' && step.type !== 'WAIT_FOR_EVENT'

  const getStepLabel = () => {
    // Format stepId: cusdec_entry -> Cusdec Entry
    return step.stepId
      .replace(/_/g, ' ')
      .replace(/\b\w/g, (c: string) => c.toUpperCase())
  }

  const handleExecute = async (e: React.MouseEvent) => {
    e.stopPropagation()
    if (!consignmentId) {
      console.error('No consignment ID found in URL')
      return
    }

    setIsLoading(true)
    try {
      await executeTask(consignmentId, step.taskId, step.type)
      navigate(`/consignments/${consignmentId}/tasks/${step.taskId}`)
    } catch (error) {
      console.error('Failed to execute task:', error)
    } finally {
      setIsLoading(false)
    }
  }

  return (
    <div
      className={`px-4 py-3 rounded-lg border-2 hover:cursor-default shadow-sm min-w-50 ${
        statusStyle.bgColor
      } ${statusStyle.borderColor} ${
        step.status === 'READY' ? 'ring-2 ring-blue-300 ring-offset-2' : ''
      }`}
    >
      <Handle
        type="target"
        position={Position.Left}
        className="bg-slate-400! w-3! h-3!"
      />

      <div className="flex items-center justify-between gap-3">
        <div>
          <div className="flex items-center justify-between mb-1">
            <div className={`flex items-center gap-2 ${statusStyle.iconColor}`}>
              {typeConfig.icon}
              <Text size="1" weight="medium" className={statusStyle.textColor}>
                {typeConfig.label}
              </Text>
            </div>
            {statusStyle.statusIcon}
          </div>
          <Text
            size="2"
            weight="bold"
            className={`${statusStyle.textColor} block`}
          >
            {getStepLabel()}
          </Text>
        </div>

        {isExecutable && (
          <button
            onClick={handleExecute}
            disabled={isLoading}
            className="flex items-center justify-center w-10 h-10 rounded-full bg-blue-500 hover:bg-blue-600 active:bg-blue-700 text-white shadow-md hover:cursor-pointer hover:shadow-lg transition-all duration-150 shrink-0 disabled:bg-slate-400 disabled:cursor-not-allowed"
            title="Execute task"
          >
            {isLoading ? (
              <UpdateIcon className="w-5 h-5 animate-spin" />
            ) : (
              <PlayIcon className="w-5 h-5 ml-0.5" />
            )}
          </button>
        )}
      </div>

      <Handle
        type="source"
        position={Position.Right}
        className="bg-slate-400! w-3! h-3!"
      />
    </div>
  )
}