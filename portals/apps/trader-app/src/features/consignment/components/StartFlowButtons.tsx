import { useState } from 'react'
import { Button, Text } from '@radix-ui/themes'
import { ConfirmDialog, Surface } from '@portals/ui'
import { useTranslation } from 'react-i18next'
import type { FlowOption } from '@/features/consignment/types.ts'

interface StartFlowButtonsProps {
  flows: FlowOption[]
  // Starts the flow (useConsignment's startFlow), rejecting if it could not.
  onStart: (templateId: string) => Promise<void>
}

// One button per flow the caller may start on the consignment (DetailDTO.startableFlows), in
// a section of their own below the workflow steps: they are services beside the consignment's
// own flow, such as an SLPA sundry payment. A started flow cannot be withdrawn, so each start
// is confirmed first.
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
    <Surface className="mt-6">
      <section aria-labelledby="start-flows-title" className="p-4 sm:p-6">
        <h2 id="start-flows-title" className="text-sm font-semibold text-foreground">
          {t('consignments.detail.flows.title')}
        </h2>
        <p className="mt-1 text-sm text-foreground-muted">{t('consignments.detail.flows.description')}</p>
        {failed && (
          <Text as="p" size="2" color="red" className="mt-3">
            {t('consignments.detail.flows.failed', { label: failed.label })}
          </Text>
        )}
        <div className="mt-4 flex items-center gap-2 flex-wrap">
          {flows.map((flow) => (
            <Button key={flow.templateId} variant="soft" size="2" onClick={() => setPending(flow)} disabled={starting}>
              {flow.label}
            </Button>
          ))}
        </div>
      </section>

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
    </Surface>
  )
}
