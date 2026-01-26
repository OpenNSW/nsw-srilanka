import { Handle, Position } from '@xyflow/react'
import type { Node, NodeProps } from '@xyflow/react'
import { Text } from '@radix-ui/themes'
import { useParams, useNavigate } from 'react-router-dom'
import { useState } from 'react'
import type {
  WorkflowStep,
  WorkflowStepType,
  WorkflowStepStatus,
} from '../../services/types/workflow'
import {
  FileTextIcon,
  GlobeIcon,
  ClockIcon,
  CheckCircledIcon,
  LockClosedIcon,
  PlayIcon,
  UpdateIcon,
} from '@radix-ui/react-icons'
import { executeTask } from '../../services/workflow'

export interface WorkflowNodeData extends Record<string, unknown> {
  step: WorkflowStep
  status: WorkflowStepStatus
}

export type WorkflowNodeType = Node<WorkflowNodeData, 'workflowStep'>

const stepTypeConfig: Record<
  WorkflowStepType,
  { label: string; icon: React.ReactNode }
> = {
  TRADER_FORM: {
    label: 'Trader Form',
    icon: <FileTextIcon className="w-4 h-4" />,
  },
  OGA_APPLICATION: {
    label: 'OGA Application',
    icon: <GlobeIcon className="w-4 h-4" />,
  },
  SYSTEM_WAIT: {
    label: 'System Wait',
    icon: <ClockIcon className="w-4 h-4" />,
  },
}

const statusConfig: Record<
  WorkflowStepStatus,
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
  ACTIVE: {
    bgColor: 'bg-blue-50',
    borderColor: 'border-blue-400',
    textColor: 'text-blue-700',
    iconColor: 'text-blue-600',
  },
  PENDING: {
    bgColor: 'bg-gray-50',
    borderColor: 'border-gray-300',
    textColor: 'text-gray-600',
    iconColor: 'text-gray-400',
  },
  LOCKED: {
    bgColor: 'bg-slate-100',
    borderColor: 'border-slate-300',
    textColor: 'text-slate-500',
    iconColor: 'text-slate-400',
    statusIcon: <LockClosedIcon className="w-3 h-3 text-slate-400" />,
  },
}

export function WorkflowNode({ data }: NodeProps<WorkflowNodeType>) {
  const { step, status } = data
  const { consignmentId } = useParams<{ consignmentId: string }>()
  const navigate = useNavigate()
  const [isLoading, setIsLoading] = useState(false)

  const typeConfig = stepTypeConfig[step.type]
  const statusStyle = statusConfig[status]

  const isExecutable =
    (status === 'ACTIVE' || status === 'PENDING') &&
    step.type !== 'SYSTEM_WAIT'

  const getStepLabel = () => {
    if (step.config.formId) {
      return step.config.formId
        .replace(/-/g, ' ')
        .replace(/\b\w/g, (c: string) => c.toUpperCase())
    }
    if (step.config.agency && step.config.service) {
      return `${step.config.agency} - ${step.config.service.replace(
        /-/g,
        ' '
      )}`
    }
    if (step.config.event) {
      return step.config.event
        .replace(/_/g, ' ')
        .toLowerCase()
        .replace(/\b\w/g, (c: string) => c.toUpperCase())
    }
    return step.id.replace(/_/g, ' ').replace(/\b\w/g, (c: string) => c.toUpperCase())
  }

  const handleExecute = async (e: React.MouseEvent) => {
    e.stopPropagation()
    if (!consignmentId) {
      console.error('No consignment ID found in URL')
      return
    }

    setIsLoading(true)
    try {
      const taskDetails = await executeTask(consignmentId, step.id)
      // Navigate to the appropriate screen based on task type
      if (taskDetails.type === 'OGA_FORM') {
        navigate(`/consignments/${consignmentId}/tasks/${step.id}`)
      }
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
        status === 'ACTIVE' ? 'ring-2 ring-blue-300 ring-offset-2' : ''
      }`}
    >
      <Handle
        type="target"
        position={Position.Left}
        className="!bg-slate-400 !w-3 !h-3"
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