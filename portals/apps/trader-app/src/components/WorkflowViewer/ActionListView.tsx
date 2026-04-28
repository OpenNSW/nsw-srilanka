import {useMemo} from 'react'
import {Badge, Box, Button, Flex, Heading, Text,} from '@radix-ui/themes'
import {CheckCircledIcon, ClockIcon, ReloadIcon, UpdateIcon,} from '@radix-ui/react-icons'
import type {WorkflowNode} from '../../services/types/consignment'
import {ActionCard} from './ActionCard'
import {CollapsibleSection} from './CollapsibleSection'

interface ActionListViewProps {
  steps: WorkflowNode[]
  consignmentId: string
  onRefresh?: () => void
  refreshing?: boolean
  className?: string
  consignmentState?: string
}

export function ActionListView({
                                 steps,
                                 consignmentId,
                                 onRefresh,
                                 refreshing = false,
                                 className = "",
                                 consignmentState
                               }: ActionListViewProps) {
  const filteredSteps = useMemo(() => {
    return steps.filter(step => {
      const type = step.workflowNodeTemplate.type?.toUpperCase();
      return type !== 'START' && type !== 'END' && type !== 'GATEWAY' && type !== 'END_NODE';
    });
  }, [steps]);

  const groups = useMemo(() => {
    return {
      active: filteredSteps.filter((s) => s.state === 'READY' || s.state === 'IN_PROGRESS'),
      upcoming: filteredSteps.filter((s) => s.state === 'LOCKED'),
      finished: filteredSteps.filter((s) => s.state === 'COMPLETED' || s.state === 'FAILED'),
    }
  }, [filteredSteps])

  const isConsignmentTerminal = consignmentState === 'FINISHED' || consignmentState === 'FAILED';

  const displaySteps = useMemo(() => {
    if (isConsignmentTerminal) {
      return filteredSteps.filter(s => s.state === 'COMPLETED' || s.state === 'FAILED');
    }
    return filteredSteps;
  }, [filteredSteps, isConsignmentTerminal]);

  const RefreshButton = (onRefresh && !isConsignmentTerminal) ? (
    <Button
      variant="soft"
      color="blue"
      size="2"
      onClick={onRefresh}
      disabled={refreshing}
      className="cursor-pointer"
    >
      <ReloadIcon className={refreshing ? 'animate-spin' : ''}/>
      Refresh
    </Button>
  ) : null;

  return (
    <div className={`w-full flex flex-col min-h-0 relative ${className}`}>
      <div className="flex-1 overflow-y-auto pr-2 custom-scrollbar min-h-0">
        {isConsignmentTerminal ? (
          <Box mb="6">
            <Flex align="center" justify="between" my="4" px="3">
              <Flex align="center" gap="2">
                <div
                  className={`w-1.5 h-5 ${consignmentState === 'FINISHED' ? 'bg-green-500' : 'bg-red-500'} rounded-full`}/>
                <Heading size="4" color={consignmentState === 'FINISHED' ? 'green' : 'red'} weight="bold">
                  Task History
                </Heading>
                <Badge color={consignmentState === 'FINISHED' ? 'green' : 'red'} variant="solid" radius="full">
                  {displaySteps.length}
                </Badge>
              </Flex>
            </Flex>
            <Box px="0.5">
              {displaySteps.map((step) => (
                <ActionCard key={step.id} step={step} consignmentId={consignmentId}/>
              ))}
            </Box>
          </Box>
        ) : (
          <>
            {groups.active.length > 0 ? (
              <Box mb="6">
                <Flex align="center" justify="between" my="4" px="3">
                  <Flex align="center" gap="2">
                    <div className="w-1.5 h-5 bg-blue-500 rounded-full"/>
                    <Heading size="4" color="blue" weight="bold">
                      Action Required
                    </Heading>
                    <Badge color="blue" variant="solid" radius="full">
                      {groups.active.length}
                    </Badge>
                  </Flex>
                  {RefreshButton}
                </Flex>
                <Box px="0.5">
                  {groups.active.map((step) => (
                    <ActionCard key={step.id} step={step} consignmentId={consignmentId}/>
                  ))}
                </Box>
              </Box>
            ) : filteredSteps.length > 0 && filteredSteps.every((s) => s.state === 'COMPLETED') ? (
              <Box py="10" px="6" mb="6"
                   className="text-center bg-green-50/50 rounded-xl border border-green-100 shadow-sm relative">
                {onRefresh && (
                  <div className="absolute top-3 right-3">
                    {RefreshButton}
                  </div>
                )}
                <div
                  className="w-16 h-16 bg-green-100 rounded-full flex items-center justify-center mx-auto mb-4 border border-green-200">
                  <CheckCircledIcon className="w-10 h-10 text-green-600"/>
                </div>
                <Heading size="4" color="green" mb="2">Process Complete</Heading>
                <Text size="3" color="green" className="opacity-80">All workflow steps have been finished successfully.
                  No further actions are required.</Text>
              </Box>
            ) : filteredSteps.length > 0 ? (
              <Box py="8" px="6" mb="6"
                   className="text-center bg-white rounded-xl border border-gray-200 border-dashed shadow-sm relative">
                {onRefresh && (
                  <div className="absolute top-3 right-3">
                    {RefreshButton}
                  </div>
                )}
                <ClockIcon className="w-12 h-12 text-slate-400 mx-auto mb-3"/>
                <Heading size="3" color="gray" mb="1">Waiting for Updates</Heading>
                <Text size="2" color="gray">Current steps are being processed. Next tasks will unlock
                  automatically.</Text>
              </Box>
            ) : null}

            <CollapsibleSection title="Upcoming Tasks" count={groups.upcoming.length}>
              {groups.upcoming.map((step) => (
                <ActionCard key={step.id} step={step} consignmentId={consignmentId}/>
              ))}
            </CollapsibleSection>

            <CollapsibleSection title="Process History" count={groups.finished.length} color="green">
              {groups.finished.map((step) => (
                <ActionCard key={step.id} step={step} consignmentId={consignmentId}/>
              ))}
            </CollapsibleSection>
          </>
        )}
      </div>

      {refreshing && (
        <Flex
          position="absolute"
          inset="0"
          align="center"
          justify="center"
          className="bg-white/60 backdrop-blur-sm z-10 rounded-lg"
        >
          <Flex direction="column" align="center" gap="3">
            <UpdateIcon className="animate-spin w-8 h-8 text-blue-600"/>
            <Text weight="medium" color="blue">Updating your list...</Text>
          </Flex>
        </Flex>
      )}
    </div>
  )
}
