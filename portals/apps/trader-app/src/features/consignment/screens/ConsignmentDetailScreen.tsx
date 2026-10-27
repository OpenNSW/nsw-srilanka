import { useCallback, useEffect, useRef, useState } from 'react'
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
import { getConsignment } from '@/features/consignment/service.ts'
import { getStateColor, formatState, formatDateTime } from '@/features/consignment/utils.ts'
import { ROUTES } from '@/constants'

type ConsignmentErrorKey = 'idRequired' | 'notFound' | 'loadFailed'

// A freshly created consignment can be returned before its workflow has been
// provisioned (workflowNodes still empty). Rather than flashing the empty
// "no workflow" state, re-fetch a bounded number of times with exponential
// backoff until the nodes appear or the consignment reaches a terminal state. The same
// backoff waits for a newly started flow's first step to appear.
const PROVISION_MAX_ATTEMPTS = 5
const PROVISION_BASE_DELAY_MS = 1000
const PROVISION_MAX_DELAY_MS = 5000

type FetchMode = 'initial' | 'refresh' | 'poll'

interface ConsignmentDetailScreenProps {
  // Defaults to the trader/CHA-facing, ownership-checked getConsignment. Pass
  // getConsignmentForAdmin (features/admin/service.ts) to reuse this screen for the
  // ConsignmentAdminRead-gated admin view instead.
  fetcher?: (consignmentId: string) => Promise<ConsignmentDetail | null>
  // The page above this one, first in the breadcrumb and where the error state's
  // buttons go. Defaults to the trader/CHA consignments list.
  parent?: BreadcrumbItem
  // Whether to offer the consignment's startableFlows. The admin view, whose callers cannot
  // start flows, turns it off.
  canStartFlows?: boolean
}

export function ConsignmentDetailScreen({
  fetcher = getConsignment,
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
  const [consignment, setConsignment] = useState<ConsignmentDetail | null>(null)
  const [loading, setLoading] = useState(!!consignmentId)
  const [refreshing, setRefreshing] = useState(false)
  const [provisioning, setProvisioning] = useState(false)
  const [error, setError] = useState<ConsignmentErrorKey | null>(consignmentId ? null : 'idRequired')
  const provisionAttemptsRef = useRef(0)
  const provisionTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null)
  const requestCountRef = useRef(0)
  // The step IDs shown when a flow was started, or null when no start is pending. Fetches
  // keep polling until a step outside this set (the new flow's first) appears.
  const stepsBeforeStartRef = useRef<Set<string> | null>(null)

  const clearProvisionTimer = useCallback(() => {
    if (provisionTimerRef.current) {
      clearTimeout(provisionTimerRef.current)
      provisionTimerRef.current = null
    }
  }, [])

  const stopAwaitingStartedFlow = useCallback(() => {
    if (stepsBeforeStartRef.current !== null) {
      stepsBeforeStartRef.current = null
      setRefreshing(false)
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
        const result = await fetcher(consignmentId)
        if (requestId !== requestCountRef.current) return

        if (!result) {
          setError('notFound')
          setProvisioning(false)
          return
        }
        setConsignment(result)
        setError(null)

        const nodes = result.workflowNodes ?? []
        const awaitingProvisioning = nodes.length === 0 && result.state === 'IN_PROGRESS'
        const stepsBeforeStart = stepsBeforeStartRef.current
        const awaitingStartedFlow = stepsBeforeStart !== null && nodes.every((node) => stepsBeforeStart.has(node.id))

        if ((awaitingProvisioning || awaitingStartedFlow) && provisionAttemptsRef.current < PROVISION_MAX_ATTEMPTS) {
          const delay = Math.min(PROVISION_BASE_DELAY_MS * 2 ** provisionAttemptsRef.current, PROVISION_MAX_DELAY_MS)
          provisionAttemptsRef.current += 1
          // A started flow is awaited on the current view, under the refresh spinner, not
          // the full-page set-up spinner.
          setProvisioning(awaitingProvisioning && !awaitingStartedFlow)
          provisionTimerRef.current = setTimeout(() => void fetchConsignmentRef.current?.('poll'), delay)
        } else {
          setProvisioning(false)
          stopAwaitingStartedFlow()
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
          stopAwaitingStartedFlow()
          setError('loadFailed')
        }
      } finally {
        if (requestId === requestCountRef.current) {
          if (mode === 'initial') setLoading(false)
          // While a started flow is awaited, the refresh spinner stays until the wait ends.
          if (mode === 'refresh' && stepsBeforeStartRef.current === null) setRefreshing(false)
        }
      }
    },
    [consignmentId, fetcher, clearProvisionTimer, stopAwaitingStartedFlow],
  )

  const handleRefresh = () => {
    // Update ref in event handler (not render / not useEffect body) so polls
    // fired during or after this refresh always call the latest callback.
    fetchConsignmentRef.current = fetchConsignment
    setRefreshing(true)
    setError(null)
    void fetchConsignment('refresh')
  }

  // A started flow's first step is created a moment after the start returns, so reload until
  // a step that was not shown before appears.
  const handleFlowStarted = () => {
    stepsBeforeStartRef.current = new Set((consignment?.workflowNodes ?? []).map((node) => node.id))
    handleRefresh()
  }

  useEffect(() => {
    if (!consignmentId) return
    clearProvisionTimer()
    let cancelled = false
    void fetcher(consignmentId)
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

        const awaitingProvisioning = (result.workflowNodes?.length ?? 0) === 0 && result.state === 'IN_PROGRESS'

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
  }, [consignmentId, fetcher, fetchConsignment, clearProvisionTimer])

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
    <div className="max-w-5xl mx-auto w-full px-4 sm:px-6 lg:px-8 pt-6 pb-10">
      <div className="flex items-center justify-between gap-4">
        <Breadcrumb items={[up, { label: consignment.name || untitled }]} />
        <Tooltip content={t('consignments.detail.refresh')}>
          <IconButton
            type="button"
            variant="soft"
            color="gray"
            onClick={handleRefresh}
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
        {canStartFlows && (
          <StartFlowButtons
            consignmentId={consignment.id}
            flows={consignment.startableFlows ?? []}
            onStarted={handleFlowStarted}
          />
        )}
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
