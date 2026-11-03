import { useState } from 'react'
import { Badge, Button } from '@radix-ui/themes'
import { ChevronRightIcon, CubeIcon, ExclamationTriangleIcon, LayersIcon } from '@radix-ui/react-icons'
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
// own flow, such as an SLPA sundry payment. Each is a card with its own Start button. A started
// flow cannot be withdrawn, so each start is confirmed first.
export function AuxiliaryServices({ flows, onStart }: AuxiliaryServicesProps) {
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
    <Surface className="mt-6 overflow-hidden">
      <section aria-labelledby="auxiliary-services-title">
        <header className="flex items-center gap-3 border-b border-border p-4 sm:p-6">
          <span className="flex-shrink-0 rounded-xl border border-info/30 bg-info-subtle p-2.5 text-info-strong">
            <LayersIcon aria-hidden width={20} height={20} />
          </span>
          <div className="flex flex-wrap items-center gap-2.5">
            <h2 id="auxiliary-services-title" className="text-lg font-semibold tracking-tight text-foreground">
              {t('consignments.detail.flows.title')}
            </h2>
            <Badge color="blue" variant="soft" radius="full">
              {t('consignments.detail.flows.available', { count: flows.length })}
            </Badge>
          </div>
        </header>

        <div className="p-4 sm:p-6">
          {failed && (
            <p className="mb-3 flex items-center gap-1 text-sm font-medium text-error-strong">
              <ExclamationTriangleIcon aria-hidden />
              {t('consignments.detail.flows.failed', { label: failed.label })}
            </p>
          )}
          <ul className="space-y-3">
            {flows.map((flow) => (
              <li
                key={flow.templateId}
                className="flex flex-col gap-4 rounded-xl border border-border bg-app-surface p-4 transition-colors hover:border-info/40 sm:flex-row sm:items-center sm:justify-between sm:p-5"
              >
                <div className="flex min-w-0 items-center gap-3.5">
                  <span className="flex h-10 w-10 flex-shrink-0 items-center justify-center rounded-lg bg-info-subtle text-info-strong">
                    <CubeIcon aria-hidden width={18} height={18} />
                  </span>
                  <span className="font-semibold text-foreground">{flow.label}</span>
                </div>
                <Button
                  size="2"
                  className="self-end sm:self-auto"
                  onClick={() => setPending(flow)}
                  disabled={starting}
                  aria-label={t('consignments.detail.flows.startService', { label: flow.label })}
                >
                  {t('consignments.detail.flows.start')}
                  <ChevronRightIcon aria-hidden />
                </Button>
              </li>
            ))}
          </ul>
          <p className="mt-4 border-t border-border pt-3 text-xs text-foreground-muted">
            {t('consignments.detail.flows.footer')}
          </p>
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
