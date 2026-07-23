import { Button } from '@radix-ui/themes'
import { CheckCircledIcon, CrossCircledIcon, ReloadIcon } from '@radix-ui/react-icons'
import { useTranslation } from 'react-i18next'
import type { ConsignmentState } from '@/features/consignment/types'

export interface ConsignmentStatusBannerProps {
  /** Current state of the consignment workflow */
  state: ConsignmentState
  /** Optional callback handler for refreshing the consignment status */
  onRefresh?: () => void
  /** Indicates whether a refresh request is currently pending */
  refreshing?: boolean
  /** Additional CSS class names for styling customization */
  className?: string
}

/**
 * ConsignmentStatusBanner renders a prominent status card at the top of the workflow viewer.
 *
 * It dynamically displays:
 * - Green Success Banner when consignment workflow state is terminal success (COMPLETED, APPROVED, FINISHED).
 * - Red Rejection/Alert Banner when consignment workflow state is terminal failure/rejection (REJECTED, SLTB_REJECTED, FAILED, DISAPPROVED).
 */
export function ConsignmentStatusBanner({
  state,
  onRefresh,
  refreshing = false,
  className = '',
}: ConsignmentStatusBannerProps) {
  const { t } = useTranslation()

  const isSuccess = state === 'FINISHED' || state === 'COMPLETED' || state === 'APPROVED'
  const isRejected = state === 'REJECTED' || state === 'SLTB_REJECTED' || state === 'FAILED' || state === 'DISAPPROVED'

  if (!isSuccess && !isRejected) {
    return null
  }

  const RefreshButton = onRefresh ? (
    <Button
      variant="soft"
      color={isSuccess ? 'green' : 'red'}
      size="2"
      onClick={onRefresh}
      disabled={refreshing}
      className="cursor-pointer"
    >
      <ReloadIcon className={refreshing ? 'animate-spin' : ''} />
      {t('workflow.refresh', 'Refresh')}
    </Button>
  ) : null

  if (isRejected) {
    return (
      <div
        className={`w-full bg-error-subtle border border-red-200 dark:border-red-900/50 rounded-xl p-6 text-center relative shadow-sm transition-all ${className}`}
      >
        {onRefresh && <div className="absolute top-4 right-4">{RefreshButton}</div>}
        <div className="w-12 h-12 rounded-full bg-red-100 dark:bg-red-900/40 border border-red-200 dark:border-red-800/50 flex items-center justify-center mx-auto mb-3 text-error-strong">
          <CrossCircledIcon className="w-7 h-7" />
        </div>
        <h2 className="text-xl font-bold text-error-strong mb-1">
          {t('workflow.banner.rejected.title', 'Consignment Rejected')}
        </h2>
        <p className="text-sm text-error-strong/90 max-w-xl mx-auto leading-relaxed">
          {t(
            'workflow.banner.rejected.subtext',
            'This consignment was rejected during review. Please inspect the process history below for rejection comments and details.',
          )}
        </p>
      </div>
    )
  }

  return (
    <div
      className={`w-full bg-success-subtle border border-green-200 dark:border-green-900/50 rounded-xl p-6 text-center relative shadow-sm transition-all ${className}`}
    >
      {onRefresh && <div className="absolute top-4 right-4">{RefreshButton}</div>}
      <div className="w-12 h-12 rounded-full bg-green-100 dark:bg-green-900/40 border border-green-200 dark:border-green-800/50 flex items-center justify-center mx-auto mb-3 text-success-strong">
        <CheckCircledIcon className="w-7 h-7" />
      </div>
      <h2 className="text-xl font-bold text-success-strong mb-1">
        {t('workflow.banner.complete.title', 'Process Complete')}
      </h2>
      <p className="text-sm text-success-strong/90 max-w-xl mx-auto leading-relaxed">
        {t(
          'workflow.banner.complete.subtext',
          'All workflow steps have been finished successfully. No further actions are required.',
        )}
      </p>
    </div>
  )
}
