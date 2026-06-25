import React from 'react'
import { useNavigate } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { Badge, Box, Card, Flex, Text } from '@radix-ui/themes'
import {
  CheckCircledIcon,
  ChevronRightIcon,
  ClockIcon,
  CrossCircledIcon,
  FileTextIcon,
  InfoCircledIcon,
  LockClosedIcon,
  PlayIcon,
  ReaderIcon,
  UpdateIcon,
} from '@radix-ui/react-icons'
import type { WorkflowNode, WorkflowNodeState } from '../../types'

const nodeTypeIcons: Record<string, React.ReactNode> = {
  SIMPLE_FORM: <FileTextIcon className="w-4 h-4" />,
  WAIT_FOR_EVENT: <ClockIcon className="w-4 h-4" />,
  PAYMENT: <ReaderIcon className="w-4 h-4" />,
  DOCUMENT_UPLOAD: <ReaderIcon className="w-4 h-4" />,
}

const statusConfig: Record<
  WorkflowNodeState,
  {
    color: 'green' | 'blue' | 'orange' | 'gray' | 'red'
    label: string
    icon: React.ReactNode
  }
> = {
  COMPLETED: {
    color: 'green',
    label: 'Completed',
    icon: <CheckCircledIcon className="w-4 h-4" />,
  },
  READY: {
    color: 'blue',
    label: 'Ready',
    icon: <PlayIcon className="w-4 h-4" />,
  },
  IN_PROGRESS: {
    color: 'orange',
    label: 'In Progress',
    icon: <UpdateIcon className="w-4 h-4" />,
  },
  LOCKED: {
    color: 'gray',
    label: 'Locked',
    icon: <LockClosedIcon className="w-3 h-3" />,
  },
  FAILED: {
    color: 'red',
    label: 'Failed',
    icon: <CrossCircledIcon className="w-4 h-4" />,
  },
}

const STATUS_KEYS: Record<WorkflowNodeState, 'completed' | 'ready' | 'inProgress' | 'locked' | 'failed'> = {
  COMPLETED: 'completed',
  READY: 'ready',
  IN_PROGRESS: 'inProgress',
  LOCKED: 'locked',
  FAILED: 'failed',
}

export interface ActionCardProps {
  step: WorkflowNode
  consignmentId: string
}

// Keyed by the Radix semantic color names in statusConfig above, mapped to
// brand status tokens (green→success, blue→info, orange→warning, red→error,
// gray→secondary). See the token block in index.css.
const statusStyles: Record<string, string> = {
  green: 'bg-success-subtle text-success-strong border-success-subtle',
  blue: 'bg-info-subtle text-info-strong border-info-subtle',
  orange: 'bg-warning-subtle text-warning-strong border-warning-subtle',
  gray: 'bg-surface text-foreground-muted border-border',
  red: 'bg-error-subtle text-error-strong border-error-subtle',
}

export const ActionCard = ({ step, consignmentId }: ActionCardProps) => {
  const navigate = useNavigate()
  const { t } = useTranslation()
  const config = statusConfig[step.state] || { color: 'gray', label: step.state, icon: null }

  const handleOpen = () => {
    void navigate(`/consignments/${consignmentId}/tasks/${step.id}`)
  }

  const label = step.workflowNodeTemplate.name || `Step ${step.id.split('-').pop()}`
  const isClickable = step.state !== 'LOCKED'

  return (
    <Card
      variant="classic"
      role={isClickable ? 'button' : undefined}
      tabIndex={isClickable ? 0 : -1}
      onClick={isClickable ? handleOpen : undefined}
      onKeyDown={
        isClickable
          ? (e) => {
              if (e.key === 'Enter' || e.key === ' ') {
                e.preventDefault()
                handleOpen()
              }
            }
          : undefined
      }
      className={`mb-3 transition-all duration-200 border shadow-sm group
        ${
          isClickable
            ? 'bg-background border-border hover:border-info/40 hover:bg-info-subtle/40 hover:shadow-md cursor-pointer active:scale-[0.98] active:shadow-sm'
            : 'bg-surface border-border opacity-50 cursor-not-allowed'
        }`}
    >
      <Flex direction="column" gap="3">
        <Flex align="center" justify="between" gap="3">
          <Flex align="center" gap="3" className="flex-1 min-w-0">
            <Box className={`p-2.5 rounded-lg border ${statusStyles[config.color] || statusStyles.gray}`}>
              {nodeTypeIcons[step.workflowNodeTemplate.type] || <FileTextIcon className="w-5 h-5" />}
            </Box>
            <Box className="flex-1 min-w-0">
              <Text size="3" weight="bold" className="block truncate text-foreground">
                {label}
              </Text>
              <Flex align="center" gap="2" mt="1">
                <Badge color={config.color} variant="soft" size="1">
                  <Flex align="center" gap="1">
                    {config.icon}
                    {t(`workflow.status.${STATUS_KEYS[step.state]}`)}
                  </Flex>
                </Badge>
              </Flex>
            </Box>
          </Flex>

          <ChevronRightIcon
            className={`flex-shrink-0 transition-colors duration-200 ${isClickable ? 'text-foreground-subtle group-hover:text-info' : 'invisible'}`}
            width="20"
            height="20"
          />
        </Flex>

        {step.workflowNodeTemplate.description && (
          <Box className="p-2 rounded">
            <Text size="2" color="gray" className="leading-relaxed">
              {step.workflowNodeTemplate.description}
            </Text>
          </Box>
        )}

        {step.extendedState && (
          <Flex align="center" gap="1" className="text-warning-strong">
            <InfoCircledIcon className="w-3.5 h-3.5" />
            <Text size="1" weight="medium" className="italic">
              {step.extendedState}
            </Text>
          </Flex>
        )}
      </Flex>
    </Card>
  )
}
