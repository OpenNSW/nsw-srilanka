import { useState, useEffect, useRef } from 'react'
import { useNavigate } from 'react-router-dom'
import { Button, Card, Heading, Text, Badge, Spinner, Flex, Box, Callout } from '@radix-ui/themes'
import { FileTextIcon, PlayIcon, EyeOpenIcon, CheckCircledIcon, ExclamationTriangleIcon } from '@radix-ui/react-icons'
import {
  getTraderPreConsignments,
  createPreConsignment,
  getPreConsignment,
  type TraderPreConsignmentItem,
} from '../services/preConsignment'
import { useApi } from '../services/ApiContext'
import { PaginationControl } from '../components/common/PaginationControl'

export function PreconsignmentScreen() {
  const navigate = useNavigate()
  const api = useApi()
  const [loading, setLoading] = useState(true)
  const [items, setItems] = useState<TraderPreConsignmentItem[]>([])
  const [totalCount, setTotalCount] = useState(0)
  const [page, setPage] = useState(0)
  const limit = 15
  const listRequestIdRef = useRef(0)

  const [notification, setNotification] = useState<{ type: 'success' | 'error'; message: string } | null>(null)

  const loadData = async () => {
    const requestId = ++listRequestIdRef.current
    try {
      setLoading(true)
      const response = await getTraderPreConsignments(page * limit, limit, api)
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
      setNotification({ type: 'error', message: 'Failed to load pre-consignments list.' })
    } finally {
      if (requestId === listRequestIdRef.current) {
        setLoading(false)
      }
    }
  }

  // Check if all dependencies for a template are completed
  const areDependenciesMet = (item: TraderPreConsignmentItem): boolean => {
    if (!item.dependsOn || item.dependsOn.length === 0) {
      return true // No dependencies
    }

    // Check if all dependent pre-consignments are completed
    return item.dependsOn.every((depId) => {
      const depItem = items.find((i) => i.id === depId)
      return depItem?.state === 'COMPLETED'
    })
  }

  useEffect(() => {
    loadData()
  }, [api, page])

  // Auto-dismiss success notifications
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
      const instance = await createPreConsignment(templateId, api)

      const nodes = instance.workflowNodes || []
      const targetNode = nodes.find(
        (node) =>
          (node.state === 'READY' || node.state === 'IN_PROGRESS') && node.workflowNodeTemplate?.type === 'SIMPLE_FORM',
      )

      if (targetNode) {
        navigate(`/pre-consignments/${instance.id}/tasks/${targetNode.id}`)
      } else {
        setNotification({ type: 'error', message: 'No ready task found in pre-consignment.' })
        setLoading(false)
      }
    } catch (error) {
      console.error('Failed to start process', error)
      setNotification({ type: 'error', message: 'Failed to start registration process.' })
      setLoading(false)
    }
  }

  const handleContinueProcess = async (preConsignmentId: string) => {
    setNotification(null)
    try {
      setLoading(true)
      const instance = await getPreConsignment(preConsignmentId, api)
      const nodes = instance.workflowNodes || []

      // Find the appropriate task
      let targetNode = nodes.find((node) => node.state === 'IN_PROGRESS' || node.state === 'READY')
      if (!targetNode && nodes.length > 0) {
        targetNode = nodes[nodes.length - 1]
      }

      if (targetNode) {
        navigate(`/pre-consignments/${instance.id}/tasks/${targetNode.id}`)
      } else {
        setNotification({ type: 'error', message: 'No task found in pre-consignment.' })
        setLoading(false)
      }
    } catch (error) {
      console.error('Failed to load process details', error)
      setNotification({ type: 'error', message: 'An error occurred while loading the process details.' })
      setLoading(false)
    }
  }

  // Render logic for notifications
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
      <Heading mb="6">Verified Documents</Heading>

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
                      onClick={() => handleStartProcess(item.id)}
                      disabled={isLocked || !areDependenciesMet(item)}
                      style={{ cursor: isLocked || !areDependenciesMet(item) ? 'not-allowed' : 'pointer' }}
                      title={!areDependenciesMet(item) ? 'Complete dependent pre-consignments first' : ''}
                    >
                      <PlayIcon /> Start
                    </Button>
                  ) : isCompleted ? (
                    <Button
                      variant="outline"
                      color="green"
                      onClick={() => handleContinueProcess(item.preConsignment!.id)}
                      style={{ cursor: 'pointer' }}
                    >
                      <EyeOpenIcon /> View
                    </Button>
                  ) : (
                    <Button
                      onClick={() => handleContinueProcess(item.preConsignment!.id)}
                      style={{ cursor: 'pointer' }}
                    >
                      Continue
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
          totalPages={Math.ceil(totalCount / limit)}
          onPageChange={(p) => setPage(p - 1)}
          hasNext={(page + 1) * limit < totalCount}
          hasPrev={page > 0}
          totalCount={totalCount}
        />
      )}
    </Box>
  )
}
