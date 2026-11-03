import { useState } from 'react'
import { Button, Select, Text } from '@radix-ui/themes'
import { ConfirmDialog, Surface } from '@portals/ui'
import { useTranslation } from 'react-i18next'
import type { FlowOption } from '@/features/consignment/types.ts'

interface AuxiliaryServicesProps {
  flows: FlowOption[]
  // Starts the flow (useConsignment's startFlow), rejecting if it could not.
  onStart: (templateId: string) => Promise<void>
}

// The flows the caller may start on the consignment (DetailDTO.startableFlows), offered in a
// section of their own below the workflow steps: they are services beside the consignment's
// own flow, such as an SLPA sundry payment. The trader picks one from a list and starts it. A
// started flow cannot be withdrawn, so each start is confirmed first.
export function AuxiliaryServices({ flows, onStart }: AuxiliaryServicesProps) {
  const { t } = useTranslation()
  const [selectedId, setSelectedId] = useState('')
  const [pending, setPending] = useState<FlowOption | null>(null)
  const [starting, setStarting] = useState(false)
  const [failed, setFailed] = useState<FlowOption | null>(null)

  if (flows.length === 0) return null

  const selected = flows.find((flow) => flow.templateId === selectedId) ?? null

  const start = async (flow: FlowOption) => {
    setStarting(true)
    setFailed(null)
    try {
      await onStart(flow.templateId)
      setPending(null)
      setSelectedId('')
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
      <section aria-labelledby="auxiliary-services-title" className="p-4 sm:p-6">
        <h2 id="auxiliary-services-title" className="text-sm font-semibold text-foreground">
          {t('consignments.detail.flows.title')}
        </h2>
        {failed && (
          <Text as="p" size="2" color="red" className="mt-3">
            {t('consignments.detail.flows.failed', { label: failed.label })}
          </Text>
        )}
        <div className="mt-4 flex flex-wrap items-center gap-2">
          <Select.Root value={selectedId} onValueChange={setSelectedId} disabled={starting}>
            <Select.Trigger
              className="min-w-64"
              placeholder={t('consignments.detail.flows.placeholder')}
              aria-label={t('consignments.detail.flows.title')}
            />
            <Select.Content>
              {flows.map((flow) => (
                <Select.Item key={flow.templateId} value={flow.templateId}>
                  {flow.label}
                </Select.Item>
              ))}
            </Select.Content>
          </Select.Root>
          <Button size="2" onClick={() => setPending(selected)} disabled={!selected || starting}>
            {t('consignments.detail.flows.start')}
          </Button>
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
