import { useState } from 'react'
import { Button, Text } from '@radix-ui/themes'
import { ConfirmDialog } from '@portals/ui'
import { useTranslation } from 'react-i18next'
import type { FlowOption } from '@/features/consignment/types.ts'

interface StartFlowButtonsProps {
  flows: FlowOption[]
  // Starts the flow (useConsignment's startFlow), rejecting if it could not.
  onStart: (templateId: string) => Promise<void>
}

// One button per flow the caller may start on the consignment (DetailDTO.startableFlows). A
// started flow cannot be withdrawn, so each start is confirmed first.
export function StartFlowButtons({ flows, onStart }: StartFlowButtonsProps) {
  const { t } = useTranslation()
  const [pending, setPending] = useState<FlowOption | null>(null)
  const [starting, setStarting] = useState(false)
  const [failed, setFailed] = useState<FlowOption | null>(null)

  if (flows.length === 0) return null

  const start = async (flow: FlowOption) => {
    setStarting(true)
    setFailed(null)
    try {
      await onStart(flow.templateId)
      setPending(null)
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

      <ConfirmDialog
        open={pending !== null}
        onOpenChange={(open) => !open && setPending(null)}
        title={t('consignments.detail.flows.confirmTitle', { label: pending?.label ?? '' })}
        description={t('consignments.detail.flows.confirmDescription', { label: pending?.label ?? '' })}
        confirmLabel={t('consignments.detail.flows.start')}
        cancelLabel={t('consignments.detail.flows.cancel')}
        onConfirm={() => pending && void start(pending)}
        busy={starting}
      />
    </div>
  )
}
