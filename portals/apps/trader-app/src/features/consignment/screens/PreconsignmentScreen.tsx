import { useState, useEffect, useRef, useCallback } from 'react'
import { useNavigate } from 'react-router-dom'
import { Button, Card, Heading, Text, Badge, Spinner, Flex, Box, Callout } from '@radix-ui/themes'
import { FileTextIcon, PlayIcon, EyeOpenIcon, CheckCircledIcon, ExclamationTriangleIcon } from '@radix-ui/react-icons'
import { useTranslation } from 'react-i18next'
import { getTraderPreConsignments, createPreConsignment, getPreConsignment } from '../service'
import { PaginationControl } from '@/components/common/PaginationControl'
import type { TraderPreConsignmentItem } from '../types'

const PAGE_LIMIT = 15

export function PreconsignmentScreen() {
  const navigate = useNavigate()
  const { t } = useTranslation()
  const [loading, setLoading] = useState(true)
  const [items, setItems] = useState<TraderPreConsignmentItem[]>([])
  const [totalCount, setTotalCount] = useState(0)
  const [page, setPage] = useState(0)
  const listRequestIdRef = useRef(0)

  const [notification, setNotification] = useState<{ type: 'success' | 'error'; message: string } | null>(null)

  const loadData = useCallback(async () => {
    const requestId = ++listRequestIdRef.current
    try {
      setLoading(true)
      const response = await getTraderPreConsignments(page * PAGE_LIMIT, PAGE_LIMIT)
      if (requestId !== listRequestIdRef.current) {
        return
      }
      setItems(response.items || [])
      setTotalCount(response.total)
    } catch (error) {
      if (requestId !== listRequestIdRef.current) {
        return
      }
      console.error('Failed to load pre-consignments', error)
      setNotification({ type: 'error', message: t('preconsignment.error.loadFailed') })
    } finally {
      if (requestId === listRequestIdRef.current) {
        setLoading(false)
      }
    }
  }, [page, t])

  const areDependenciesMet = (item: TraderPreConsignmentItem): boolean => {
    if (!item.dependsOn || item.dependsOn.length === 0) {
      return true
    }
    return item.dependsOn.every((depId) => {
      const depItem = items.find((i) => i.id === depId)
      return depItem?.state === 'COMPLETED'
    })
  }

  useEffect(() => {
    // eslint-disable-next-line react-hooks/set-state-in-effect
    void loadData()
  }, [loadData])

  useEffect(() => {
    if (notification?.type === 'success') {
      const timer = setTimeout(() => setNotification(null), 5000)
      return () => clearTimeout(timer)
    }
  }, [notification])

  const handleStartProcess = async (templateId: string) => {
    setNotification(null)
    try {
      setLoading(true)
      const instance = await createPreConsignment(templateId)

      const nodes = instance.workflowNodes || []
      const targetNode = nodes.find(
        (node) =>
          (node.state === 'READY' || node.state === 'IN_PROGRESS') && node.workflowNodeTemplate?.type === 'SIMPLE_FORM',
      )

      if (targetNode) {
        void navigate(`/pre-consignments/${instance.id}/tasks/${targetNode.id}`)
      } else {
        setNotification({ type: 'error', message: t('preconsignment.error.noReadyTask') })
        setLoading(false)
      }
    } catch (error) {
      console.error('Failed to start process', error)
      setNotification({ type: 'error', message: t('preconsignment.error.startFailed') })
      setLoading(false)
    }
  }

  const handleContinueProcess = async (preConsignmentId: string) => {
    setNotification(null)
    try {
      setLoading(true)
      const instance = await getPreConsignment(preConsignmentId)
      const nodes = instance.workflowNodes || []

      let targetNode = nodes.find((node) => node.state === 'IN_PROGRESS' || node.state === 'READY')
      if (!targetNode && nodes.length > 0) {
        targetNode = nodes[nodes.length - 1]
      }

      if (targetNode) {
        void navigate(`/pre-consignments/${instance.id}/tasks/${targetNode.id}`)
      } else {
        setNotification({ type: 'error', message: t('preconsignment.error.noTask') })
        setLoading(false)
      }
    } catch (error) {
      console.error('Failed to load process details', error)
      setNotification({ type: 'error', message: t('preconsignment.error.loadDetailFailed') })
      setLoading(false)
    }
  }

  const renderNotification = () => {
    if (!notification) return null
    return (
      <Callout.Root color={notification.type === 'success' ? 'green' : 'red'} mb="4">
        <Callout.Icon>
          {notification.type === 'success' ? <CheckCircledIcon /> : <ExclamationTriangleIcon />}
        </Callout.Icon>
        <Callout.Text>{notification.message}</Callout.Text>
      </Callout.Root>
    )
  }

  if (loading) {
    return (
      <Flex align="center" justify="center" style={{ height: '50vh' }}>
        <Spinner size="3" />
      </Flex>
    )
  }

  return (
    <Box p="6">
      <Heading mb="6">{t('preconsignment.title')}</Heading>

      {renderNotification()}

      <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
        {items.map((item) => {
          const hasInstance = !!item.preConsignment
          const isCompleted = item.state === 'COMPLETED'
          const isLocked = item.state === 'LOCKED'
          const isInProgress = item.state === 'IN_PROGRESS' || (hasInstance && !isCompleted)

          return (
            <Card key={item.id} size="2" style={{ position: 'relative' }}>
              <Flex direction="column" gap="3">
                <Flex justify="between" align="start">
                  <Box>
                    <Heading size="4" mb="1">
                      {item.name}
                    </Heading>
                    <Text size="2" color="gray">
                      {item.description}
                    </Text>
                  </Box>
                  <FileTextIcon width="24" height="24" className="text-foreground-subtle" />
                </Flex>

                <Flex justify="between" align="center" mt="4">
                  <Badge color={isCompleted ? 'green' : isInProgress ? 'blue' : isLocked ? 'gray' : 'orange'}>
                    {item.state.replace('_', ' ')}
                  </Badge>

                  {!hasInstance ? (
                    <Button
                      onClick={() => {
                        void handleStartProcess(item.id)
                      }}
                      disabled={isLocked || !areDependenciesMet(item)}
                      style={{ cursor: isLocked || !areDependenciesMet(item) ? 'not-allowed' : 'pointer' }}
                      title={!areDependenciesMet(item) ? 'Complete dependent pre-consignments first' : ''}
                    >
                      <PlayIcon /> {t('preconsignment.action.start')}
                    </Button>
                  ) : isCompleted ? (
                    <Button
                      variant="outline"
                      color="green"
                      onClick={() => {
                        void handleContinueProcess(item.preConsignment!.id)
                      }}
                      style={{ cursor: 'pointer' }}
                    >
                      <EyeOpenIcon /> {t('preconsignment.action.view')}
                    </Button>
                  ) : (
                    <Button
                      onClick={() => {
                        void handleContinueProcess(item.preConsignment!.id)
                      }}
                      style={{ cursor: 'pointer' }}
                    >
                      {t('preconsignment.action.continue')}
                    </Button>
                  )}
                </Flex>
              </Flex>
            </Card>
          )
        })}
      </div>
      {items.length > 0 && (
        <PaginationControl
          currentPage={page + 1}
          totalPages={Math.ceil(totalCount / PAGE_LIMIT)}
          onPageChange={(p) => setPage(p - 1)}
          hasNext={(page + 1) * PAGE_LIMIT < totalCount}
          hasPrev={page > 0}
          totalCount={totalCount}
        />
      )}
    </Box>
  )
}
