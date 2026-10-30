import { useParams, useNavigate } from 'react-router-dom'
import { Button, IconButton, Spinner, Text, Tooltip } from '@radix-ui/themes'
import { ArrowLeftIcon, ReloadIcon } from '@radix-ui/react-icons'
import { useTranslation } from 'react-i18next'
import { ClaimBanner } from './ClaimBanner'
import { useTask, type ActionErrorKey, type ClaimErrorKey } from './hooks'
import { TraderZoneLayout } from '@/features/zone/components/TraderZoneLayout'
import { zoneWidthClass } from '@/features/zone/width'
import { Breadcrumb, type BreadcrumbItem } from '@/components/Breadcrumb'
import { humanizeStatus } from '@/utils/formatStatus'
import { isAgencyMode } from '@/features/case/mode'
import { ROUTES } from '@/constants'
import { LoadError } from '@/components/common/LoadError.tsx'

export function TaskDetailScreen() {
  const { taskId, consignmentId } = useParams<{ taskId: string; consignmentId: string }>()
  const navigate = useNavigate()
  const goToTasks = () => {
    if (consignmentId) {
      void navigate(ROUTES.CONSIGNMENT_DETAIL(consignmentId), { replace: true })
      return
    }
    void navigate(-1)
  }
  const { t } = useTranslation()
  const {
    zoneView,
    consignment,
    loading,
    error,
    refreshing,
    refetch,
    formEpoch,
    submitted,
    actionError,
    claimBusy,
    claimError,
    changeClaim,
    saveDraft,
    submit,
  } = useTask(taskId, consignmentId)

  const claimErrorMessages: Record<ClaimErrorKey, string> = {
    completed: t('tasks.claim.error.completed'),
    changed: t('tasks.claim.error.changed'),
    notHolder: t('tasks.claim.error.notHolder'),
    unavailable: t('tasks.claim.error.unavailable'),
    failed: t('tasks.claim.error.failed'),
  }
  const actionErrorMessages: Record<ActionErrorKey, string> = {
    staleStep: t('tasks.error.staleStep'),
    draftFailed: t('tasks.error.draftFailed'),
    submitFailed: t('tasks.error.submitFailed'),
  }
  const submitError = actionError && actionErrorMessages[actionError]

  if (loading) {
    return (
      <div className="flex justify-center items-center h-full p-6">
        <Spinner size="3" />
        <Text size="3" color="gray" className="ml-3">
          {t('tasks.loading')}
        </Text>
      </div>
    )
  }

  if (error) {
    return (
      <div className="p-6">
        <div className="bg-app-surface rounded-2xl shadow-md p-8 flex flex-col items-center gap-4">
          <LoadError message={t('tasks.error.fetchFailed')} onRetry={refetch} />
          <Button variant="soft" color="gray" onClick={goToTasks}>
            <ArrowLeftIcon />
            {t('tasks.goBack')}
          </Button>
        </div>
      </div>
    )
  }

  if (!zoneView) {
    return (
      <div className="p-6">
        <div className="bg-app-surface rounded-2xl shadow-md p-6 text-center">
          <Text size="4" color="gray" weight="medium">
            {t('tasks.error.notFound')}
          </Text>
          <div className="mt-4">
            <Button variant="soft" onClick={goToTasks}>
              <ArrowLeftIcon />
              {t('tasks.goBack')}
            </Button>
          </div>
        </div>
      </div>
    )
  }

  const width = zoneWidthClass(zoneView)
  // The task's configured name, or its type until the consignment has loaded.
  const title =
    consignment?.workflowNodes?.find((node) => node.id === taskId)?.workflowNodeTemplate.name ||
    humanizeStatus(zoneView.task_type)
  const breadcrumb: BreadcrumbItem[] = [
    { label: t(isAgencyMode ? 'nav.cases' : 'nav.consignments'), to: ROUTES.CONSIGNMENTS },
  ]
  if (consignmentId) {
    breadcrumb.push({
      label: consignment?.name || t(isAgencyMode ? 'cases.detail.title' : 'consignments.detail.title'),
      to: ROUTES.CONSIGNMENT_DETAIL(consignmentId),
    })
  }
  breadcrumb.push({ label: title })

  return (
    <div className="min-h-full">
      <div className={`${width} mx-auto px-4 sm:px-6 lg:px-8 pt-6 flex items-center justify-between gap-4`}>
        <Breadcrumb items={breadcrumb} />
        <Tooltip content={t('tasks.refresh')}>
          <IconButton
            type="button"
            variant="soft"
            color="gray"
            onClick={refetch}
            disabled={refreshing}
            aria-label={t('tasks.refresh')}
          >
            <ReloadIcon className={refreshing ? 'animate-spin' : ''} />
          </IconButton>
        </Tooltip>
      </div>
      {submitError && (
        <div className={`${width} mx-auto px-4 sm:px-6 lg:px-8 pt-4`}>
          <div className="rounded-xl bg-error-subtle px-4 py-3 shadow-sm">
            <Text size="2" weight="medium" className="text-error-strong">
              {submitError}
            </Text>
          </div>
        </div>
      )}
      {zoneView.claim && (
        <div className={`${width} mx-auto px-4 sm:px-6 lg:px-8 pt-4`}>
          <ClaimBanner
            claim={zoneView.claim}
            completed={zoneView.state === 'COMPLETED'}
            busy={claimBusy}
            error={claimError && claimErrorMessages[claimError]}
            onClaim={() => void changeClaim('claim')}
            onRelease={() => void changeClaim('release')}
          />
        </div>
      )}
      <TraderZoneLayout
        task={zoneView}
        title={title}
        formEpoch={formEpoch}
        onSubmitForm={
          submitted ? undefined : (command, data, options) => (options?.draft ? saveDraft(data) : submit(command, data))
        }
      />
      {zoneView.state === 'COMPLETED' && (
        <div className={`${width} mx-auto px-4 sm:px-6 lg:px-8 pb-8 flex justify-end`}>
          <Button size="3" variant="soft" onClick={goToTasks}>
            {t(isAgencyMode ? 'tasks.backToCase' : 'tasks.backToConsignment')}
          </Button>
        </div>
      )}
    </div>
  )
}
