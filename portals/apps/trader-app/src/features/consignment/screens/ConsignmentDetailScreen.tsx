import { useParams, useNavigate } from 'react-router-dom'
import { Button } from '@radix-ui/themes'
import { ArrowLeftIcon } from '@radix-ui/react-icons'
import { useTranslation } from 'react-i18next'
import { isAgencyMode } from '@/features/case/mode'
import { ConsignmentTasks } from '@/features/consignment/components/WorkflowViewer'
import { StartFlowButtons } from '@/features/consignment/components/StartFlowButtons'
import type { ConsignmentDetail } from '@/features/consignment/types.ts'
import { useConsignment } from '@/features/consignment/hooks.ts'
import { formatDateTime } from '@/features/consignment/utils.ts'
import { ROUTES } from '@/constants'
import { ConsignmentStateBadge } from '@/features/consignment/components/ConsignmentStateBadge'
import { TradeFlowBadge } from '@/features/consignment/components/TradeFlowBadge'
import {
  Breadcrumb,
  type BreadcrumbItem,
  CopyableId,
  LoadingOverlay,
  PageState,
  RefreshButton,
  Surface,
} from '@portals/ui'
import { Can } from '@/components/Can'

interface ConsignmentDetailScreenProps {
  // Keeps this view's results apart from other views of the same consignment.
  scope?: string
  // Defaults to the trader/CHA-facing, ownership-checked getConsignment. Pass
  // getConsignmentForAdmin (features/admin/service.ts) to reuse this screen for the
  // ConsignmentAdminRead-gated admin view instead.
  fetcher?: (consignmentId: string, signal?: AbortSignal) => Promise<ConsignmentDetail | null>
  // The page above this one, first in the breadcrumb and where the error state's
  // buttons go. Defaults to the trader/CHA consignments list.
  parent?: BreadcrumbItem
  // Whether to offer the consignment's startableFlows. The admin view, whose callers cannot
  // start flows, turns it off.
  canStartFlows?: boolean
}

export function ConsignmentDetailScreen({
  scope,
  fetcher,
  parent,
  canStartFlows = true,
}: ConsignmentDetailScreenProps = {}) {
  const { consignmentId } = useParams<{ consignmentId: string }>()
  const navigate = useNavigate()
  const { t } = useTranslation()
  // An agency's officers work on cases: the same records, under their own name.
  const up = parent ?? { label: t(isAgencyMode ? 'nav.cases' : 'nav.consignments'), to: ROUTES.CONSIGNMENTS }
  const untitled = t(isAgencyMode ? 'cases.detail.title' : 'consignments.detail.title')
  const backTo = up.to ?? ROUTES.CONSIGNMENTS
  const { consignment, error, loading, provisioning, refreshing, refetch, startFlow } = useConsignment(consignmentId, {
    scope,
    fetcher,
  })

  if (loading || provisioning) {
    const message = provisioning
      ? t('consignments.detail.loading.settingUp')
      : consignment
        ? t('consignments.detail.loading.consignment')
        : t('consignments.detail.loading.processing')
    return (
      <div className="p-6">
        <PageState variant="loading" title={message} />
      </div>
    )
  }

  if (error || !consignment) {
    const isLoadFailed = error === 'loadFailed'
    const errorKey = error ?? 'notFound'
    const errorTitle =
      errorKey === 'loadFailed'
        ? t('consignments.detail.error.loadFailed')
        : errorKey === 'idRequired'
          ? t('consignments.detail.error.idRequired')
          : t('consignments.detail.error.notFound')

    return (
      <div className="p-6">
        <div className="mb-6">
          <Breadcrumb items={[up, { label: untitled }]} />
        </div>
        <Surface>
          <PageState
            variant="error"
            title={errorTitle}
            description={
              isLoadFailed
                ? t('consignments.detail.error.loadFailedDescription')
                : t('consignments.detail.error.notFoundDescription')
            }
            onRetry={isLoadFailed ? refetch : undefined}
            actions={
              <Button variant="soft" color="gray" onClick={() => void navigate(backTo)}>
                <ArrowLeftIcon />
                {t(isAgencyMode ? 'cases.detail.backToList' : 'consignments.detail.backToList')}
              </Button>
            }
          />
        </Surface>
      </div>
    )
  }

  const workflowNodes = consignment.workflowNodes || []

  return (
    <div className="max-w-5xl mx-auto w-full px-4 sm:px-6 lg:px-8 pt-6 pb-10">
      <div className="flex items-center justify-between gap-4">
        <Breadcrumb items={[up, { label: consignment.name || untitled }]} />
        <RefreshButton label={t('consignments.detail.refresh')} refreshing={refreshing} onClick={refetch} />
      </div>

      <div className="mt-10 mb-8 flex flex-wrap items-start justify-between gap-4">
        <div className="min-w-0">
          <div className="flex flex-wrap items-center gap-3">
            <h1 className="text-3xl font-semibold tracking-tight text-foreground">{consignment.name || untitled}</h1>
            <ConsignmentStateBadge state={consignment.state} size="2" />
            {/* A case carries no flow. */}
            {consignment.flow && <TradeFlowBadge flow={consignment.flow} />}
          </div>
          <p className="mt-1 flex flex-wrap items-center gap-x-4 gap-y-1 text-xs text-foreground-muted">
            <CopyableId
              label={t(isAgencyMode ? 'cases.detail.field.caseId' : 'consignments.detail.field.consignmentId')}
              id={consignment.id}
            />
            <span>
              {t('consignments.detail.field.dateCreated')} {formatDateTime(consignment.createdAt)}
            </span>
          </p>
        </div>
        {canStartFlows && (
          <Can I="consignment.startFlow">
            <StartFlowButtons flows={consignment.startableFlows ?? []} onStart={startFlow} />
          </Can>
        )}
      </div>

      <Surface className="relative">
        {refreshing && <LoadingOverlay message={t('consignments.detail.refreshing')} />}

        <div className="p-4 sm:p-6">
          {workflowNodes.length > 0 ? (
            <ConsignmentTasks
              key={`${consignment.id}-${consignment.state}`}
              steps={workflowNodes}
              consignmentId={consignmentId!}
              consignmentState={consignment.state}
            />
          ) : (
            <PageState
              variant="empty"
              title={t('consignments.detail.noWorkflow.title')}
              description={t('consignments.detail.noWorkflow.description')}
            />
          )}
        </div>
      </Surface>
    </div>
  )
}
