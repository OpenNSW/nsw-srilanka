import { useState } from 'react'
import { AlertDialog, Button, Flex, Text } from '@radix-ui/themes'
import { useTranslation } from 'react-i18next'
import type { FlowOption } from '@/features/consignment/types.ts'
import { startConsignmentFlow } from '@/features/consignment/service.ts'

interface StartFlowButtonsProps {
  consignmentId: string
  flows: FlowOption[]
  // Called once a flow has started, to reload the consignment so its steps show.
  onStarted: () => void
}

// One button per flow the caller may start on the consignment (DetailDTO.startableFlows). A
// started flow cannot be withdrawn, so each start is confirmed first.
export function StartFlowButtons({ consignmentId, flows, onStarted }: StartFlowButtonsProps) {
  const { t } = useTranslation()
  const [pending, setPending] = useState<FlowOption | null>(null)
  const [starting, setStarting] = useState(false)
  const [failed, setFailed] = useState<FlowOption | null>(null)

  if (flows.length === 0) return null

  const start = async (flow: FlowOption) => {
    setStarting(true)
    setFailed(null)
    try {
      await startConsignmentFlow(consignmentId, flow.templateId)
      setPending(null)
      onStarted()
    } catch (err) {
      console.error('Failed to start flow:', err)
      setPending(null)
      setFailed(flow)
    } finally {
      setStarting(false)
    }
  }

  return (
    <div className="flex items-center gap-2 flex-wrap">
      {failed && (
        <Text size="2" color="red">
          {t('consignments.detail.flows.failed', { label: failed.label })}
        </Text>
      )}
      {flows.map((flow) => (
        <Button key={flow.templateId} variant="soft" size="2" onClick={() => setPending(flow)} disabled={starting}>
          {flow.label}
        </Button>
      ))}

      <AlertDialog.Root open={pending !== null} onOpenChange={(open) => !open && !starting && setPending(null)}>
        <AlertDialog.Content maxWidth="450px">
          <AlertDialog.Title>
            {t('consignments.detail.flows.confirmTitle', { label: pending?.label ?? '' })}
          </AlertDialog.Title>
          <AlertDialog.Description size="2">
            {t('consignments.detail.flows.confirmDescription', { label: pending?.label ?? '' })}
          </AlertDialog.Description>
          <Flex gap="3" mt="4" justify="end">
            <AlertDialog.Cancel>
              <Button variant="soft" color="gray" disabled={starting}>
                {t('consignments.detail.flows.cancel')}
              </Button>
            </AlertDialog.Cancel>
            <Button loading={starting} onClick={() => pending && void start(pending)}>
              {t('consignments.detail.flows.start')}
            </Button>
          </Flex>
        </AlertDialog.Content>
      </AlertDialog.Root>
    </div>
  )
}
