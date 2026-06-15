import { useState, useEffect, useCallback } from 'react'
import { useParams, useNavigate } from 'react-router-dom'
import { Button, Spinner, Text } from '@radix-ui/themes'
import { ArrowLeftIcon, ReloadIcon } from '@radix-ui/react-icons'
import { useTranslation } from 'react-i18next'
import { getZoneView, submitTaskStep } from '../services/task'
import { useApi } from '../services/ApiContext'
import { TraderZoneLayout } from '../zones/TraderZoneLayout'
import type { ZoneView } from '../zones/types'

const POST_SUBMIT_REFETCH_DELAY_MS = 1500

export function TaskDetailScreen() {
  const { taskId } = useParams<{ taskId: string }>()
  const navigate = useNavigate()
  const goBack = () => navigate(-1)
  const api = useApi()
  const { t } = useTranslation()
  const [zoneView, setZoneView] = useState<ZoneView | null>(null)
  const [loading, setLoading] = useState(true)
  const [refreshing, setRefreshing] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const fetchTask = useCallback(
    async (mode: 'initial' | 'refresh' = 'initial') => {
      if (!taskId) {
        setError(t('tasks.error.missingId'))
        setLoading(false)
        return
      }

      try {
        if (mode === 'refresh') setRefreshing(true)
        else setLoading(true)
        setError(null)

        const zv = await getZoneView(taskId, api)
        setZoneView(zv)
      } catch (err) {
        setError(t('tasks.error.fetchFailed'))
        console.error(err)
      } finally {
        if (mode === 'refresh') setRefreshing(false)
        else setLoading(false)
      }
    },
    [api, taskId, t],
  )

  useEffect(() => {
    void fetchTask()
  }, [fetchTask])

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
        <div className="bg-background rounded-lg shadow p-6 text-center">
          <Text size="4" color="red" weight="medium">
            {error}
          </Text>
          <div className="mt-4">
            <Button variant="soft" onClick={goBack}>
              <ArrowLeftIcon />
              {t('tasks.goBack')}
            </Button>
          </div>
        </div>
      </div>
    )
  }

  if (!zoneView) {
    return (
      <div className="p-6">
        <div className="bg-background rounded-lg shadow p-6 text-center">
          <Text size="4" color="gray" weight="medium">
            {t('tasks.error.notFound')}
          </Text>
          <div className="mt-4">
            <Button variant="soft" onClick={goBack}>
              <ArrowLeftIcon />
              {t('tasks.goBack')}
            </Button>
          </div>
        </div>
      </div>
    )
  }

  return (
    <div className="bg-surface min-h-full">
      <div className="max-w-6xl mx-auto px-4 sm:px-6 lg:px-8 pt-6 flex items-center justify-between">
        <Button variant="ghost" color="gray" onClick={goBack}>
          <ArrowLeftIcon />
          {t('tasks.back')}
        </Button>
        <Button
          variant="soft"
          color="blue"
          size="2"
          onClick={() => void fetchTask('refresh')}
          disabled={refreshing}
          className="cursor-pointer"
        >
          <ReloadIcon className={refreshing ? 'animate-spin' : ''} />
          {t('tasks.refresh')}
        </Button>
      </div>
      <TraderZoneLayout
        task={zoneView}
        onSubmitForm={async (_command, data) => {
          if (!taskId) return
          try {
            await submitTaskStep(taskId, data, api)
            await new Promise((resolve) => setTimeout(resolve, POST_SUBMIT_REFETCH_DELAY_MS))
            await fetchTask()
          } catch (err) {
            setError(t('tasks.error.submitFailed'))
            console.error(err)
          }
        }}
      />
    </div>
  )
}
