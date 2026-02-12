import {useState} from 'react'
import {Handle, Position} from '@xyflow/react'
import type {Node, NodeProps} from '@xyflow/react'
import {Text, Tooltip} from '@radix-ui/themes'
import {useParams, useNavigate} from 'react-router-dom'
import type {WorkflowNode as WorkflowNodeDataType, WorkflowNodeState} from '../../services/types/consignment'

import {
  CheckCircledIcon,
  LockClosedIcon,
  PlayIcon,
  UpdateIcon,
  FileTextIcon,
  ClockIcon,
  ReaderIcon,
} from '@radix-ui/react-icons'

export interface WorkflowNodeData extends Record<string, unknown> {
  step: WorkflowNodeDataType
}

export type WorkflowNodeType = Node<WorkflowNodeData, 'workflowStep'>

const nodeTypeIcons: Record<string, React.ReactNode> = {
  SIMPLE_FORM: <FileTextIcon className="w-4 h-4"/>,
  WAIT_FOR_EVENT: <ClockIcon className="w-4 h-4"/>,
  DOCUMENT_UPLOAD: <ReaderIcon className="w-4 h-4"/>,
}

const statusConfig: Record<
  WorkflowNodeState,
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
    statusIcon: <CheckCircledIcon className="w-4 h-4 text-emerald-600"/>,
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
    statusIcon: <LockClosedIcon className="w-3 h-3 text-slate-400"/>,
  },
  REJECTED: {
    bgColor: 'bg-red-50',
    borderColor: 'border-red-400',
    textColor: 'text-red-700',
    iconColor: 'text-red-600',
  },
}

export function WorkflowNode({data}: NodeProps<WorkflowNodeType>) {
  const {step} = data
  const {consignmentId} = useParams<{ consignmentId: string }>()
  const navigate = useNavigate()
  const [isLoading, setIsLoading] = useState(false)

  const statusStyle = statusConfig[step.state] || {
    bgColor: 'bg-gray-50',
    borderColor: 'border-gray-300',
    textColor: 'text-gray-500',
    iconColor: 'text-gray-400'
  }

  const isExecutable = step.state === 'READY'
  const isViewable = step.state !== 'LOCKED' && !isExecutable

  const getViewButtonColors = () => {
    switch (step.state) {
      case 'COMPLETED':
        return 'bg-emerald-500 hover:bg-emerald-600 active:bg-emerald-700'
      case 'IN_PROGRESS':
        return 'bg-orange-500 hover:bg-orange-600 active:bg-orange-700'
      case 'REJECTED':
        return 'bg-red-500 hover:bg-red-600 active:bg-red-700'
      default:
        return 'bg-slate-500 hover:bg-slate-600 active:bg-slate-700'
    }
  }

  const getStepLabel = () => {
    // Use workflow node template name if available, otherwise use node ID
    if (step.workflowNodeTemplate.name) {
      return step.workflowNodeTemplate.name
    }
    // Extract the last segment of the node ID for a short label
    const parts = step.id.split('-')
    const lastPart = parts[parts.length - 1]
    return `Step ${lastPart}`
  }

  const getTooltipContent = () => {
    const label = getStepLabel()
    const description = step.workflowNodeTemplate.description

    if (description && description.trim()) {
      return `${label} - ${description}`
    }
    return label
  }

  const handleOpen = async (e: React.MouseEvent) => {
    e.stopPropagation()
    if (!consignmentId) {
      console.error('No consignment ID found in URL')
      return
    }

    setIsLoading(true)
    try {
      navigate(`/consignments/${consignmentId}/tasks/${step.id}`)
    } catch (error) {
      console.error('Failed to execute task:', error)
    } finally {
      setIsLoading(false)
    }
  }

  return (
    <div
      className={`px-3 py-2 rounded-lg border-2 hover:cursor-default shadow-sm w-56 ${statusStyle.bgColor
      } ${statusStyle.borderColor} ${step.state === 'READY' ? 'ring-2 ring-blue-300 ring-offset-2' : ''
      }`}
    >
      <Handle
        type="target"
        position={Position.Left}
        className="bg-slate-400! w-3! h-3!"
      />
      <div className="flex items-center justify-between gap-2">
        <div className="flex items-start gap-2 flex-1 min-w-0">
          <div className={`mt-0.5 shrink-0 ${statusStyle.iconColor}`}>
            {nodeTypeIcons[step.workflowNodeTemplate.type] || <FileTextIcon className="w-3.5 h-3.5"/>}
          </div>
          <div className="min-w-0">
            <Tooltip content={getTooltipContent()}>
              <Text
                size="1"
                weight="bold"
                className={`${statusStyle.textColor} block cursor-pointer truncate`}
              >
                {getStepLabel()}
              </Text>
            </Tooltip>
            <div>

              <Text size="1" className={`${statusStyle.textColor} font-mono mt-0.5 text-xs`}>
                {step.state}
              </Text>
            </div>
            <div>
              {step.state === "IN_PROGRESS" &&
                  <div className={`${statusStyle.textColor} mt-0.5 text-[0.5rem] italic`}>
                    {step.extendedState}
                  </div>
              }
            </div>
          </div>
        </div>

        {isExecutable && (
          <button
            onClick={handleOpen}
            disabled={isLoading}
            className="flex items-center justify-center w-8 h-8 rounded-full bg-blue-500 hover:bg-blue-600 active:bg-blue-700 text-white shadow-md hover:cursor-pointer hover:shadow-lg transition-all duration-150 shrink-0 disabled:bg-slate-400 disabled:cursor-not-allowed"
            title="Execute task"
          >
            {isLoading ? (
              <UpdateIcon className="w-4 h-4 animate-spin"/>
            ) : (
              <PlayIcon className="w-4 h-4 ml-0.5"/>
            )}
          </button>
        )}

        {isViewable && (
          <button
            onClick={handleOpen}
            disabled={isLoading}
            className={`flex items-center justify-center w-8 h-8 rounded-full ${getViewButtonColors()} text-white shadow-md hover:cursor-pointer hover:shadow-lg transition-all duration-150 shrink-0 disabled:bg-slate-400 disabled:cursor-not-allowed`}
            title="View task"
          >
            {isLoading ? (
              <UpdateIcon className="w-4 h-4 animate-spin"/>
            ) : (
              <ReaderIcon className="w-4 h-4"/>
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