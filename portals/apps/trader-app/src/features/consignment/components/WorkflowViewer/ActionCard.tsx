import React from 'react'
import { useNavigate } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { Badge, Box, Card, Flex, Text } from '@radix-ui/themes'
import {
  ChatBubbleIcon,
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
import type { WorkflowNode } from '@/features/consignment/types'
import { getNodeStatusAppearance, isLockedNodeState, type NodeStatusColor } from '@/features/consignment/utils'

const nodeTypeIcons: Record<string, React.ReactNode> = {
  SIMPLE_FORM: <FileTextIcon className="w-4 h-4" />,
  WAIT_FOR_EVENT: <ClockIcon className="w-4 h-4" />,
  PAYMENT: <ReaderIcon className="w-4 h-4" />,
  DOCUMENT_UPLOAD: <ReaderIcon className="w-4 h-4" />,
}

const statusIcons: Record<string, React.ReactNode> = {
  COMPLETED: <CheckCircledIcon className="w-4 h-4" />,
  READY: <PlayIcon className="w-4 h-4" />,
  IN_PROGRESS: <UpdateIcon className="w-4 h-4" />,
  PENDING_USER: <UpdateIcon className="w-4 h-4" />,
  PENDING_FEEDBACK: <ChatBubbleIcon className="w-4 h-4" />,
  QUEUED_EXTERNALLY: <ClockIcon className="w-4 h-4" />,
  PENDING_PAYMENT: <ReaderIcon className="w-4 h-4" />,
  LOCKED: <LockClosedIcon className="w-3 h-3" />,
  FAILED: <CrossCircledIcon className="w-4 h-4" />,
}

export interface ActionCardProps {
  step: WorkflowNode
  consignmentId: string
}

// Keyed by the Radix semantic color names in getNodeStatusAppearance, mapped to
// brand status tokens (green→success, blue→info, orange→warning, red→error,
// gray→secondary). See the token block in index.css.
const statusStyles: Record<NodeStatusColor, string> = {
  green: 'bg-success-subtle text-success-strong border-success-subtle',
  blue: 'bg-info-subtle text-info-strong border-info-subtle',
  orange: 'bg-warning-subtle text-warning-strong border-warning-subtle',
  gray: 'bg-app-bg text-foreground-muted border-border',
  red: 'bg-error-subtle text-error-strong border-error-subtle',
}

export const ActionCard = ({ step, consignmentId }: ActionCardProps) => {
  const navigate = useNavigate()
  const { t } = useTranslation()
  const appearance = getNodeStatusAppearance(step.state)
  const icon = statusIcons[step.state] ?? <UpdateIcon className="w-4 h-4" />

  const handleOpen = () => void navigate(`/consignments/${consignmentId}/tasks/${step.id}`)

  const label = step.workflowNodeTemplate.name || `Step ${step.id.split('-').pop()}`
  const isClickable = !isLockedNodeState(step.state)
  const statusLabel = appearance.labelKey ? t(`workflow.status.${appearance.labelKey}`) : appearance.fallbackLabel

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
            ? 'bg-app-surface border-border hover:border-info/40 hover:bg-info-subtle/40 hover:shadow-md cursor-pointer active:scale-[0.98] active:shadow-sm'
            : 'bg-app-bg border-border opacity-50 cursor-not-allowed'
        }`}
    >
      <Flex direction="column" gap="3">
        <Flex align="center" justify="between" gap="3">
          <Flex align="center" gap="3" className="flex-1 min-w-0">
            <Box className={`p-2.5 rounded-lg border ${statusStyles[appearance.color] || statusStyles.gray}`}>
              {nodeTypeIcons[step.workflowNodeTemplate.type] || <FileTextIcon className="w-5 h-5" />}
            </Box>
            <Box className="flex-1 min-w-0">
              <Text size="3" weight="bold" className="block truncate text-foreground">
                {label}
              </Text>
              <Flex align="center" gap="2" mt="1">
                <Badge color={appearance.color} variant="soft" size="1">
                  <Flex align="center" gap="1">
                    {icon}
                    {statusLabel}
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
