import { useParams, useNavigate } from 'react-router-dom'
import { Button, Badge, IconButton, Spinner, Text, Tooltip } from '@radix-ui/themes'
import { ArrowLeftIcon, ReloadIcon } from '@radix-ui/react-icons'
import { useTranslation } from 'react-i18next'
import { Breadcrumb, type BreadcrumbItem } from '@/components/Breadcrumb'
import { CopyableId } from '@/components/CopyableId'
import { isAgencyMode } from '@/features/case/mode'
import { ConsignmentTasks } from '@/features/consignment/components/WorkflowViewer'
import { StartFlowButtons } from '@/features/consignment/components/StartFlowButtons'
import type { ConsignmentDetail } from '@/features/consignment/types.ts'
import { useConsignment } from '@/features/consignment/hooks.ts'
import { getStateColor, formatState, formatDateTime } from '@/features/consignment/utils.ts'
import { ROUTES } from '@/constants'

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
        <div className="flex items-center justify-center py-12">
          <Spinner size="3" />
          <Text size="3" color="gray" className="ml-3">
            {message}
          </Text>
        </div>
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
        <div className="bg-app-surface rounded-2xl shadow-md p-8 text-center">
          <Text size="5" color="red" weight="medium" className="block mb-2">
            {errorTitle}
          </Text>
          <Text size="2" color="gray" className="block mb-6">
            {isLoadFailed
              ? t('consignments.detail.error.loadFailedDescription')
              : t('consignments.detail.error.notFoundDescription')}
          </Text>
          <div className="flex gap-3 justify-center">
            <Button variant="soft" onClick={() => void navigate(backTo)}>
              <ArrowLeftIcon />
              {t(isAgencyMode ? 'cases.detail.backToList' : 'consignments.detail.backToList')}
            </Button>
            {isLoadFailed && <Button onClick={refetch}>{t('consignments.detail.tryAgain')}</Button>}
          </div>
        </div>
      </div>
    )
  }

  const workflowNodes = consignment.workflowNodes || []

  return (
    <div className="max-w-5xl mx-auto w-full px-4 sm:px-6 lg:px-8 pt-6 pb-10">
      <div className="flex items-center justify-between gap-4">
        <Breadcrumb items={[up, { label: consignment.name || untitled }]} />
        <Tooltip content={t('consignments.detail.refresh')}>
          <IconButton
            type="button"
            variant="soft"
            color="gray"
            onClick={refetch}
            disabled={refreshing}
            aria-label={t('consignments.detail.refresh')}
          >
            <ReloadIcon className={refreshing ? 'animate-spin' : ''} />
          </IconButton>
        </Tooltip>
      </div>

      <div className="mt-10 mb-8 flex flex-wrap items-start justify-between gap-4">
        <div className="min-w-0">
          <div className="flex flex-wrap items-center gap-3">
            <h1 className="text-3xl font-semibold tracking-tight text-foreground">{consignment.name || untitled}</h1>
            <Badge size="2" color={getStateColor(consignment.state)}>
              {formatState(consignment.state)}
            </Badge>
            {/* A case carries no flow. */}
            {consignment.flow && (
              <Badge size="1" color={consignment.flow === 'IMPORT' ? 'blue' : 'green'} variant="soft">
                {consignment.flow === 'IMPORT'
                  ? t('consignments.tradeFlow.import')
                  : t('consignments.tradeFlow.export')}
              </Badge>
            )}
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
        {canStartFlows && <StartFlowButtons flows={consignment.startableFlows ?? []} onStart={startFlow} />}
      </div>

      <div className="bg-app-surface rounded-2xl shadow-md relative">
        {refreshing && (
          <div className="absolute inset-0 bg-app-surface/80 backdrop-blur-sm z-20 flex items-center justify-center rounded-lg">
            <div className="flex items-center gap-3 bg-app-surface px-6 py-4 rounded-xl shadow-lg">
              <Spinner size="3" />
              <Text size="3" weight="medium" color="gray">
                {t('consignments.detail.refreshing')}
              </Text>
            </div>
          </div>
        )}

        <div className="p-4 sm:p-6">
          {workflowNodes.length > 0 ? (
            <ConsignmentTasks
              key={`${consignment.id}-${consignment.state}`}
              steps={workflowNodes}
              consignmentId={consignmentId!}
              consignmentState={consignment.state}
            />
          ) : (
            <div className="flex items-center justify-center py-12">
              <div className="text-center">
                <Text size="4" color="gray" weight="medium" className="block mb-2">
                  {t('consignments.detail.noWorkflow.title')}
                </Text>
                <Text size="2" color="gray">
                  {t('consignments.detail.noWorkflow.description')}
                </Text>
              </div>
            </div>
          )}
        </div>
      </div>
    </div>
  )
}
