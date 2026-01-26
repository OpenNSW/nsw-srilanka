import { useState, useEffect } from 'react'
import { Dialog, Button, Tabs, Box, Flex, Text, Spinner, ScrollArea, IconButton } from '@radix-ui/themes'
import { Cross2Icon } from '@radix-ui/react-icons'
import { WorkflowCard } from './WorkflowCard'
import { HSCodeSearch } from './HSCodeSearch'
import type {HSCode} from "../../services/types/hsCode.ts";
import type {Workflow} from "../../services/types/workflow.ts";
import {getWorkflowsByHSCode} from "../../services/workflow.ts";

interface HSCodePickerProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  onSelect: (hsCode: HSCode, workflow: Workflow) => void
  /** Dialog title */
  title?: string
  /** Dialog description */
  description?: string
  /** Confirm button text */
  confirmText?: string
  /** Cancel button text */
  cancelText?: string
}

export function HSCodePicker({
  open,
  onOpenChange,
  onSelect,
  title = 'Select HS Code & Workflow',
  description = 'Search for an HS Code and select a workflow to proceed.',
  confirmText = 'Confirm',
  cancelText = 'Cancel',
}: HSCodePickerProps) {
  const [selectedHSCode, setSelectedHSCode] = useState<HSCode | null>(null)
  const [selectedWorkflow, setSelectedWorkflow] = useState<Workflow | null>(null)
  const [importWorkflows, setImportWorkflows] = useState<Workflow[]>([])
  const [exportWorkflows, setExportWorkflows] = useState<Workflow[]>([])
  const [loadingWorkflows, setLoadingWorkflows] = useState(false)

  useEffect(() => {
    async function fetchWorkflows() {
      if (!selectedHSCode) {
        setImportWorkflows([])
        setExportWorkflows([])
        return
      }

      setLoadingWorkflows(true)
      setSelectedWorkflow(null)

      try {
        const result = await getWorkflowsByHSCode({ hs_code: selectedHSCode.code })
        setImportWorkflows(result.import)
        setExportWorkflows(result.export)
      } catch (error) {
        console.error('Failed to fetch workflows:', error)
        setImportWorkflows([])
        setExportWorkflows([])
      } finally {
        setLoadingWorkflows(false)
      }
    }

    fetchWorkflows()
  }, [selectedHSCode])

  const handleConfirm = () => {
    if (selectedHSCode && selectedWorkflow) {
      onSelect(selectedHSCode, selectedWorkflow)
      onOpenChange(false)
      resetState()
    }
  }

  const resetState = () => {
    setSelectedHSCode(null)
    setSelectedWorkflow(null)
    setImportWorkflows([])
    setExportWorkflows([])
  }

  const handleOpenChange = (isOpen: boolean) => {
    if (!isOpen) {
      resetState()
    }
    onOpenChange(isOpen)
  }

  const hasWorkflows = importWorkflows.length > 0 || exportWorkflows.length > 0

  return (
    <Dialog.Root open={open} onOpenChange={handleOpenChange}>
      <Dialog.Content
        maxWidth="600px"
        style={{ minHeight: '500px', display: 'flex', flexDirection: 'column' }}
        onInteractOutside={(e) => e.preventDefault()}
      >
        <Flex justify="between" align="start">
          <Box>
            <Dialog.Title>{title}</Dialog.Title>
            <Dialog.Description size="2" color="gray">
              {description}
            </Dialog.Description>
          </Box>
          <Dialog.Close>
            <IconButton variant="ghost" color="gray" size="1">
              <Cross2Icon />
            </IconButton>
          </Dialog.Close>
        </Flex>

        <Box mt="4" />

        <Box style={{ flex: 1 }}>
          <Box mb="5">
            <HSCodeSearch value={selectedHSCode} onChange={setSelectedHSCode} />
          </Box>

          {selectedHSCode && (
            <Box mb="4">
              {loadingWorkflows ? (
                <Flex align="center" justify="center" py="6">
                  <Spinner size="2" />
                  <Text size="2" color="gray" ml="2">
                    Loading workflows...
                  </Text>
                </Flex>
              ) : hasWorkflows ? (
                <Tabs.Root defaultValue="import">
                  <Tabs.List>
                    <Tabs.Trigger value="import">Import ({importWorkflows.length})</Tabs.Trigger>
                    <Tabs.Trigger value="export">Export ({exportWorkflows.length})</Tabs.Trigger>
                  </Tabs.List>

                  <Box pt="3">
                    <Tabs.Content value="import">
                      {importWorkflows.length > 0 ? (
                        <ScrollArea style={{ maxHeight: '250px' }}>
                          <Flex direction="column" gap="2">
                            {importWorkflows.map((workflow) => (
                              <WorkflowCard
                                key={workflow.id}
                                workflow={workflow}
                                selected={selectedWorkflow?.id === workflow.id}
                                onSelect={setSelectedWorkflow}
                              />
                            ))}
                          </Flex>
                        </ScrollArea>
                      ) : (
                        <Text size="2" color="gray">No import workflow available</Text>
                      )}
                    </Tabs.Content>

                    <Tabs.Content value="export">
                      {exportWorkflows.length > 0 ? (
                        <ScrollArea style={{ maxHeight: '250px' }}>
                          <Flex direction="column" gap="2">
                            {exportWorkflows.map((workflow) => (
                              <WorkflowCard
                                key={workflow.id}
                                workflow={workflow}
                                selected={selectedWorkflow?.id === workflow.id}
                                onSelect={setSelectedWorkflow}
                              />
                            ))}
                          </Flex>
                        </ScrollArea>
                      ) : (
                        <Text size="2" color="gray">No export workflow available</Text>
                      )}
                    </Tabs.Content>
                  </Box>
                </Tabs.Root>
              ) : (
                <Flex align="center" justify="center" py="6" direction="column" gap="1">
                  <Text size="2" color="gray">
                    No workflows available for this HS Code
                  </Text>
                  <Text size="1" color="gray">
                    Please select a more specific HS Code (6-digit level)
                  </Text>
                </Flex>
              )}
            </Box>
          )}

          {/* Selection Summary */}
          {selectedHSCode && selectedWorkflow && (
            <Box p="3" className="bg-blue-50 border border-blue-200 rounded-lg">
              <Text size="2" weight="medium" className="text-blue-900 block mb-2">
                Selection Summary
              </Text>
              <Flex direction="column" gap="1">
                <Flex gap="2">
                  <Text size="1" color="gray" style={{ minWidth: '80px' }}>HS Code:</Text>
                  <Text size="1" weight="medium">{selectedHSCode.code}</Text>
                </Flex>
                <Flex gap="2">
                  <Text size="1" color="gray" style={{ minWidth: '80px' }}>Description:</Text>
                  <Text size="1" className="text-gray-700">{selectedHSCode.description}</Text>
                </Flex>
                <Flex gap="2">
                  <Text size="1" color="gray" style={{ minWidth: '80px' }}>Workflow:</Text>
                  <Text size="1" weight="medium">{selectedWorkflow.name}</Text>
                </Flex>
                <Flex gap="2">
                  <Text size="1" color="gray" style={{ minWidth: '80px' }}>Type:</Text>
                  <Text size="1" weight="medium" style={{ textTransform: 'capitalize' }}>
                    {selectedWorkflow.type}
                  </Text>
                </Flex>
              </Flex>
            </Box>
          )}
        </Box>

        <Flex gap="3" justify="end" mt="4">
          <Dialog.Close>
            <Button variant="soft" color="gray">
              {cancelText}
            </Button>
          </Dialog.Close>
          <Button
            onClick={handleConfirm}
            disabled={!selectedHSCode || !selectedWorkflow}
          >
            {confirmText}
          </Button>
        </Flex>
      </Dialog.Content>
    </Dialog.Root>
  )
}