import { useState } from 'react'
import { Badge, Button } from '@radix-ui/themes'
import { ChevronDownIcon, ChevronRightIcon, CubeIcon, ExclamationTriangleIcon, LayersIcon } from '@radix-ui/react-icons'
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
// own flow, such as an SLPA sundry payment. Each is a card with its own Start button, and the
// list folds away under its header. A started flow cannot be withdrawn, so each start is
// confirmed first.
export function AuxiliaryServices({ flows, onStart }: AuxiliaryServicesProps) {
  const { t } = useTranslation()
  const [open, setOpen] = useState(false)
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
        {/* The header opens and closes the list, an accordion that starts closed. Its text is
            sized as the steps card's headings are. */}
        <h2 id="auxiliary-services-title">
          <button
            type="button"
            aria-expanded={open}
            aria-controls="auxiliary-services-list"
            onClick={() => setOpen((o) => !o)}
            className="group flex w-full items-center gap-2 p-4 text-left text-sm font-semibold text-foreground transition-colors hover:bg-app-surface sm:px-6"
          >
            <LayersIcon aria-hidden className="text-info-strong" />
            <span>{t('consignments.detail.flows.title')}</span>
            <Badge color="gray" variant="soft" radius="full">
              {t('consignments.detail.flows.available', { count: flows.length })}
            </Badge>
            <ChevronDownIcon
              aria-hidden
              className={`ml-auto flex-shrink-0 text-foreground-muted transition-transform duration-200 group-hover:text-foreground ${
                open ? 'rotate-180' : ''
              }`}
            />
          </button>
        </h2>

        <div id="auxiliary-services-list" hidden={!open} className="border-t border-border p-4 sm:p-6">
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
                className="flex flex-wrap items-center justify-between gap-3 rounded-xl border border-border bg-app-surface px-4 py-3 transition-colors hover:border-info/40"
              >
                <div className="flex min-w-0 items-center gap-2">
                  <CubeIcon aria-hidden className="flex-shrink-0 text-info-strong" />
                  <span className="font-semibold text-foreground">{flow.label}</span>
                </div>
                <Button
                  size="2"
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
          <p className="mt-6 text-sm text-foreground-muted">{t('consignments.detail.flows.footer')}</p>
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
