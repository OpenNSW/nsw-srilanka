import { useState } from 'react'
import { Dialog, Button, Box, Flex, Text, IconButton, Badge } from '@radix-ui/themes'
import { Cross2Icon, ArrowRightIcon } from '@radix-ui/react-icons'
import { HSCodeSearch } from './HSCodeSearch'
import type { HSCode } from '../../services/types/hsCode.ts'
import type { TradeFlow } from '../../services/types/consignment.ts'

interface HSCodePickerProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  onSelect: (hsCode: HSCode, tradeFlow: TradeFlow) => void
  /** Whether a consignment is being created */
  isCreating?: boolean
  /** Dialog title */
  title?: string
  /** Confirm button text */
  confirmText?: string
  /** Cancel button text */
  cancelText?: string
  /** If provided, skips trade-flow step and uses this trade flow */
  fixedTradeFlow?: TradeFlow
}

export function HSCodePicker({
  open,
  onOpenChange,
  onSelect,
  isCreating = false,
  title = 'New Consignment',
  confirmText = 'Start Consignment',
  cancelText = 'Cancel',
  fixedTradeFlow,
}: HSCodePickerProps) {
  const [step, setStep] = useState<'trade-flow' | 'hs-code'>(fixedTradeFlow ? 'hs-code' : 'trade-flow')
  const [tradeFlow, setTradeFlow] = useState<TradeFlow | null>(fixedTradeFlow ?? null)
  const [selectedHSCode, setSelectedHSCode] = useState<HSCode | null>(null)

  const handleConfirm = () => {
    if (selectedHSCode && tradeFlow) {
      onSelect(selectedHSCode, tradeFlow)
      onOpenChange(false)
      resetState()
    }
  }

  const handleTradeFlowSelect = (flow: TradeFlow) => {
    setTradeFlow(flow)
    setStep('hs-code')
  }

  const handleBack = () => {
    if (step === 'hs-code') {
      if (!fixedTradeFlow) {
        setStep('trade-flow')
        setSelectedHSCode(null)
      }
    }
  }

  const resetState = () => {
    setStep(fixedTradeFlow ? 'hs-code' : 'trade-flow')
    setTradeFlow(fixedTradeFlow ?? null)
    setSelectedHSCode(null)
  }

  const handleOpenChange = (isOpen: boolean) => {
    if (!isOpen) {
      resetState()
    }
    onOpenChange(isOpen)
  }

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
              {step === 'trade-flow'
                ? 'Select whether this is an import or export consignment.'
                : `Search and select an HS code for your ${tradeFlow?.toLowerCase()} consignment.`}
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
          {step === 'trade-flow' ? (
            <Flex direction="column" gap="3">
              <Text size="2" weight="medium" color="gray">
                Select Trade Flow
              </Text>
              <Flex direction="column" gap="3">
                <button
                  onClick={() => handleTradeFlowSelect('IMPORT')}
                  className="p-6 border-2 border-border rounded-lg hover:border-info hover:bg-info-subtle transition-all text-left group cursor-pointer"
                >
                  <Flex align="center" justify="between">
                    <Box>
                      <Text size="4" weight="bold" className="text-foreground block mb-1">
                        Import
                      </Text>
                      <Text size="2" color="gray">
                        Bringing goods into the country
                      </Text>
                    </Box>
                    <ArrowRightIcon className="text-foreground-subtle group-hover:text-info" width="20" height="20" />
                  </Flex>
                </button>
                <button
                  onClick={() => handleTradeFlowSelect('EXPORT')}
                  className="p-6 border-2 border-border rounded-lg hover:border-success hover:bg-success-subtle transition-all text-left group cursor-pointer"
                >
                  <Flex align="center" justify="between">
                    <Box>
                      <Text size="4" weight="bold" className="text-foreground block mb-1">
                        Export
                      </Text>
                      <Text size="2" color="gray">
                        Sending goods out of the country
                      </Text>
                    </Box>
                    <ArrowRightIcon
                      className="text-foreground-subtle group-hover:text-success"
                      width="20"
                      height="20"
                    />
                  </Flex>
                </button>
              </Flex>
            </Flex>
          ) : (
            <>
              {/* Step indicator */}
              <Flex align="center" gap="2" mb="4">
                <Badge color={tradeFlow === 'IMPORT' ? 'blue' : 'green'} size="2">
                  {tradeFlow}
                </Badge>
                <Text size="1" color="gray">
                  Selected Trade Flow
                </Text>
              </Flex>

              {/* HS Code Search */}
              <Box mb="5">
                <HSCodeSearch value={selectedHSCode} onChange={setSelectedHSCode} />
              </Box>

              {/* HS Code Details */}
              {selectedHSCode && (
                <Box p="4" className="bg-info-subtle border border-info-subtle rounded-lg">
                  <Text size="2" weight="bold" className="text-info-strong block mb-3">
                    Selected HS Code
                  </Text>
                  <Flex direction="column" gap="2">
                    <Flex gap="2">
                      <Text size="2" color="gray" style={{ minWidth: '100px' }}>
                        HS Code:
                      </Text>
                      <Text size="2" weight="medium">
                        {selectedHSCode.hsCode}
                      </Text>
                    </Flex>
                    <Flex gap="2">
                      <Text size="2" color="gray" style={{ minWidth: '100px' }}>
                        Description:
                      </Text>
                      <Text size="2" className="text-foreground-muted" style={{ flex: 1 }}>
                        {selectedHSCode.description}
                      </Text>
                    </Flex>
                    <Flex gap="2">
                      <Text size="2" color="gray" style={{ minWidth: '100px' }}>
                        Trade Flow:
                      </Text>
                      <Text size="2" weight="medium" style={{ textTransform: 'uppercase' }}>
                        {tradeFlow}
                      </Text>
                    </Flex>
                  </Flex>
                </Box>
              )}
            </>
          )}
        </Box>

        <Flex gap="3" justify="end" mt="4">
          {step === 'hs-code' && !fixedTradeFlow && (
            <Button variant="soft" color="gray" onClick={handleBack} disabled={isCreating}>
              Back
            </Button>
          )}
          <Dialog.Close>
            <Button variant="soft" color="gray" disabled={isCreating}>
              {cancelText}
            </Button>
          </Dialog.Close>
          {step === 'hs-code' && (
            <Button onClick={handleConfirm} disabled={!selectedHSCode || isCreating} loading={isCreating}>
              {isCreating ? 'Creating...' : confirmText}
            </Button>
          )}
        </Flex>
      </Dialog.Content>
    </Dialog.Root>
  )
}
