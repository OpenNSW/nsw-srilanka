import { useState } from 'react'
import { Badge, Button, Select } from '@radix-ui/themes'
import { ChevronRightIcon, ExclamationTriangleIcon, LayersIcon } from '@radix-ui/react-icons'
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
      {/* Headed and laid out like the workflow steps card above it. */}
      <section aria-labelledby="auxiliary-services-title" className="p-4 sm:p-6">
        <h2
          id="auxiliary-services-title"
          className="mb-3 flex items-center gap-2 text-sm font-semibold text-foreground"
        >
          <LayersIcon aria-hidden className="text-info-strong" />
          {t('consignments.detail.flows.title')}
          <Badge color="gray" variant="soft" radius="full">
            {flows.length}
          </Badge>
        </h2>
        {failed && (
          <p className="mb-3 flex items-center gap-1 text-sm font-medium text-error-strong">
            <ExclamationTriangleIcon aria-hidden />
            {t('consignments.detail.flows.failed', { label: failed.label })}
          </p>
        )}
        <div className="flex flex-col gap-3 rounded-xl border border-border bg-app-surface px-4 py-3 sm:flex-row sm:items-center">
          <Select.Root value={selectedId} onValueChange={setSelectedId} disabled={starting}>
            <Select.Trigger
              className="w-full sm:flex-1"
              placeholder={t('consignments.detail.flows.placeholder')}
              aria-label={t('consignments.detail.flows.title')}
            />
            <Select.Content position="popper">
              {flows.map((flow) => (
                <Select.Item key={flow.templateId} value={flow.templateId}>
                  {flow.label}
                </Select.Item>
              ))}
            </Select.Content>
          </Select.Root>
          <Button size="2" onClick={() => setPending(selected)} disabled={!selected || starting}>
            {t('consignments.detail.flows.start')}
            <ChevronRightIcon aria-hidden />
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
