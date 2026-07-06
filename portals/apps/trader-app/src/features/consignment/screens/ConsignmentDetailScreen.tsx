import { useCallback, useEffect, useRef, useState } from 'react'
import { useParams, useNavigate } from 'react-router-dom'
import { Button, Badge, Spinner, Text } from '@radix-ui/themes'
import { ArrowLeftIcon, ReloadIcon } from '@radix-ui/react-icons'
import { useTranslation } from 'react-i18next'
import { ActionListView } from '@/features/consignment/components/WorkflowViewer'
import type { ConsignmentDetail } from '@/features/consignment/types.ts'
import { getConsignment } from '@/features/consignment/service.ts'
import { getStateColor, formatState, formatDateTime } from '@/features/consignment/utils.ts'

type ConsignmentErrorKey = 'idRequired' | 'notFound' | 'loadFailed'

// A freshly created consignment can be returned before its workflow has been
// provisioned (workflowNodes still empty). Rather than flashing the empty
// "no workflow" state, re-fetch a bounded number of times with exponential
// backoff until the nodes appear or the consignment reaches a terminal state.
const PROVISION_MAX_ATTEMPTS = 5
const PROVISION_BASE_DELAY_MS = 1000
const PROVISION_MAX_DELAY_MS = 5000

type FetchMode = 'initial' | 'refresh' | 'poll'

export function ConsignmentDetailScreen() {
  const { consignmentId } = useParams<{ consignmentId: string }>()
  const navigate = useNavigate()
  const { t } = useTranslation()
  const [consignment, setConsignment] = useState<ConsignmentDetail | null>(null)
  const [loading, setLoading] = useState(!!consignmentId)
  const [refreshing, setRefreshing] = useState(false)
  const [provisioning, setProvisioning] = useState(false)
  const [error, setError] = useState<ConsignmentErrorKey | null>(consignmentId ? null : 'idRequired')
  const provisionAttemptsRef = useRef(0)
  const provisionTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null)
  const requestCountRef = useRef(0)

  const clearProvisionTimer = useCallback(() => {
    if (provisionTimerRef.current) {
      clearTimeout(provisionTimerRef.current)
      provisionTimerRef.current = null
    }
  }, [])

  // Defined before fetchConsignment so the callback can schedule polls without
  // a self-reference, which would be a temporal dead zone access.
  const fetchConsignmentRef = useRef<((mode: FetchMode) => Promise<void>) | null>(null)

  const fetchConsignment = useCallback(
    async (mode: FetchMode = 'initial') => {
      if (!consignmentId) return

      clearProvisionTimer()
      // Track this request so stale responses (e.g. a manual refresh fired
      // while a poll is still pending) can be ignored, preventing duplicate
      // concurrent polling loops.
      const requestId = ++requestCountRef.current

      // A poll continuation keeps the attempt counter and the current view; an
      // initial load or a manual refresh starts a fresh provisioning cycle.
      if (mode !== 'poll') {
        provisionAttemptsRef.current = 0
      }

      try {
        const result = await getConsignment(consignmentId)
        if (requestId !== requestCountRef.current) return

        if (!result) {
          setError('notFound')
          setProvisioning(false)
          return
        }
        setConsignment(result)
        setError(null)

        const awaitingProvisioning =
          (result.workflowNodes?.length ?? 0) === 0 &&
          (result.state === 'INITIALIZED' || result.state === 'IN_PROGRESS')

        if (awaitingProvisioning && provisionAttemptsRef.current < PROVISION_MAX_ATTEMPTS) {
          const delay = Math.min(PROVISION_BASE_DELAY_MS * 2 ** provisionAttemptsRef.current, PROVISION_MAX_DELAY_MS)
          provisionAttemptsRef.current += 1
          setProvisioning(true)
          provisionTimerRef.current = setTimeout(() => void fetchConsignmentRef.current?.('poll'), delay)
        } else {
          setProvisioning(false)
        }
      } catch (err) {
        if (requestId !== requestCountRef.current) return
        console.error('Failed to fetch consignment:', err)

        // A transient failure mid-provisioning should keep retrying with
        // backoff rather than dropping the user onto a misleading
        // "no workflow" or error screen. Only surface the error once retries
        // are exhausted.
        if (mode === 'poll' && provisionAttemptsRef.current < PROVISION_MAX_ATTEMPTS) {
          const delay = Math.min(PROVISION_BASE_DELAY_MS * 2 ** provisionAttemptsRef.current, PROVISION_MAX_DELAY_MS)
          provisionAttemptsRef.current += 1
          provisionTimerRef.current = setTimeout(() => void fetchConsignmentRef.current?.('poll'), delay)
        } else {
          setProvisioning(false)
          setError('loadFailed')
        }
      } finally {
        if (requestId === requestCountRef.current) {
          if (mode === 'initial') setLoading(false)
          if (mode === 'refresh') setRefreshing(false)
        }
      }
    },
    [consignmentId, clearProvisionTimer],
  )

  const handleRefresh = () => {
    // Update ref in event handler (not render / not useEffect body) so polls
    // fired during or after this refresh always call the latest callback.
    fetchConsignmentRef.current = fetchConsignment
    setRefreshing(true)
    setError(null)
    void fetchConsignment('refresh')
  }

  useEffect(() => {
    if (!consignmentId) return
    clearProvisionTimer()
    let cancelled = false
    void getConsignment(consignmentId)
      .then((result) => {
        if (cancelled) return
        provisionAttemptsRef.current = 0
        if (!result) {
          setError('notFound')
          setProvisioning(false)
          setLoading(false)
          return
        }
        setConsignment(result)
        setError(null)

        const awaitingProvisioning =
          (result.workflowNodes?.length ?? 0) === 0 &&
          (result.state === 'INITIALIZED' || result.state === 'IN_PROGRESS')

        if (awaitingProvisioning && provisionAttemptsRef.current < PROVISION_MAX_ATTEMPTS) {
          const delay = Math.min(PROVISION_BASE_DELAY_MS * 2 ** provisionAttemptsRef.current, PROVISION_MAX_DELAY_MS)
          provisionAttemptsRef.current += 1
          setProvisioning(true)
          // Set ref inside async callback — satisfies react-hooks/refs and
          // react-hooks/immutability which only check the effect body, not
          // async continuations.
          fetchConsignmentRef.current = fetchConsignment
          provisionTimerRef.current = setTimeout(() => void fetchConsignmentRef.current?.('poll'), delay)
        } else {
          setProvisioning(false)
        }
        setLoading(false)
      })
      .catch((err) => {
        if (cancelled) return
        console.error('Failed to fetch consignment:', err)
        setError('loadFailed')
        setProvisioning(false)
        setLoading(false)
      })
    return () => {
      cancelled = true
      clearProvisionTimer()
    }
  }, [consignmentId, fetchConsignment, clearProvisionTimer])

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
          <Button variant="ghost" color="gray" onClick={() => void navigate('/consignments')}>
            <ArrowLeftIcon />
            {t('consignments.detail.back')}
          </Button>
        </div>
        <div className="bg-background rounded-lg shadow p-8 text-center">
          <Text size="5" color="red" weight="medium" className="block mb-2">
            {errorTitle}
          </Text>
          <Text size="2" color="gray" className="block mb-6">
            {isLoadFailed
              ? t('consignments.detail.error.loadFailedDescription')
              : t('consignments.detail.error.notFoundDescription')}
          </Text>
          <div className="flex gap-3 justify-center">
            <Button variant="soft" onClick={() => void navigate('/consignments')}>
              <ArrowLeftIcon />
              {t('consignments.detail.backToList')}
            </Button>
            {isLoadFailed && (
              <Button
                onClick={() => {
                  fetchConsignmentRef.current = fetchConsignment
                  setLoading(true)
                  setError(null)
                  void fetchConsignment('initial')
                }}
              >
                {t('consignments.detail.tryAgain')}
              </Button>
            )}
          </div>
        </div>
      </div>
    )
  }

  const workflowNodes = consignment.workflowNodes || []

  return (
    <div className="p-4 md:p-6 h-[calc(100vh-64px)] flex flex-col">
      <div className="mb-3 flex items-center justify-between">
        <Button
          variant="ghost"
          color="gray"
          onClick={() => void navigate('/consignments')}
          aria-label="Back to consignments list"
        >
          <ArrowLeftIcon />
          {t('consignments.detail.back')}
        </Button>
        <Button
          variant="soft"
          color="blue"
          size="2"
          onClick={handleRefresh}
          disabled={refreshing}
          className="cursor-pointer"
        >
          <ReloadIcon className={refreshing ? 'animate-spin' : ''} />
          {t('consignments.detail.refresh')}
        </Button>
      </div>

      <div className="mb-3 mt-2 flex items-center gap-3">
        <h1 className="text-xl font-semibold text-foreground">{consignment.name || t('consignments.detail.title')}</h1>
        <Badge size="2" color={getStateColor(consignment.state)}>
          {formatState(consignment.state)}
        </Badge>
        <Badge size="1" color={consignment.flow === 'IMPORT' ? 'blue' : 'green'} variant="soft">
          {consignment.flow}
        </Badge>
      </div>

      <div className="mb-4 md:mb-6 flex items-start gap-10">
        <div>
          <p className="text-xs font-semibold text-foreground-subtle mb-0.5">
            {t('consignments.detail.field.consignmentId')}
          </p>
          <p className="text-xs font-mono text-foreground-muted">{consignment.id}</p>
        </div>
        <div>
          <p className="text-xs font-semibold text-foreground-subtle mb-0.5">
            {t('consignments.detail.field.dateCreated')}
          </p>
          <p className="text-xs text-foreground-muted">{formatDateTime(consignment.createdAt)}</p>
        </div>
      </div>

      <div className="bg-background rounded-lg shadow flex flex-col flex-1 min-h-0 relative">
        {refreshing && (
          <div className="absolute inset-0 bg-background/80 backdrop-blur-sm z-20 flex items-center justify-center rounded-lg">
            <div className="flex items-center gap-3 bg-background px-6 py-4 rounded-lg shadow-lg">
              <Spinner size="3" />
              <Text size="3" weight="medium" color="gray">
                {t('consignments.detail.refreshing')}
              </Text>
            </div>
          </div>
        )}

        <div className="p-4 flex-1 flex flex-col min-h-0">
          {workflowNodes.length > 0 ? (
            <div className="flex-1 min-h-0 flex flex-col overflow-hidden">
              <ActionListView
                className="flex-1 min-h-0"
                steps={workflowNodes}
                consignmentId={consignmentId!}
                onRefresh={handleRefresh}
                refreshing={refreshing}
                consignmentState={consignment.state}
              />
            </div>
          ) : (
            <div className="flex-1 flex items-center justify-center">
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
