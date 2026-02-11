import { useState, useEffect } from 'react'
import { useParams, useNavigate } from 'react-router-dom'
import { Button, Spinner, Text } from '@radix-ui/themes'
import { ArrowLeftIcon } from '@radix-ui/react-icons'
import {getTaskInfo} from '../services/task'
import PluginRenderer, {type RenderInfo} from '../plugins'


export function TaskDetailScreen() {
  const { consignmentId, taskId } = useParams<{
    consignmentId: string
    taskId: string
  }>()
  const navigate = useNavigate()
  const [renderInfo, setRenderInfo] = useState<RenderInfo | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    async function fetchTask() {
      if (!taskId) {
        setError('Task ID is missing.')
        setLoading(false)
        return
      }

      try {
        setLoading(true)
        const response = await getTaskInfo(taskId)

        if (response.success && response.data) {
          setRenderInfo(response.data)
        } else {
          setError('Failed to fetch task.')
        }
      } catch (err) {
        setError('Failed to fetch task details.')
        console.error(err)
      } finally {
        setLoading(false)
      }
    }

    fetchTask()
  }, [consignmentId, taskId])


  if (loading) {
    return (
      <div className="flex justify-center items-center h-full p-6">
        <Spinner size="3" />
        <Text size="3" color="gray" className="ml-3">
          Loading task...
        </Text>
      </div>
    )
  }

  if (error) {
    return (
      <div className="p-6">
        <div className="bg-white rounded-lg shadow p-6 text-center">
          <Text size="4" color="red" weight="medium">
            {error}
          </Text>
          <div className="mt-4">
            <Button variant="soft" onClick={() => navigate(-1)}>
              <ArrowLeftIcon />
              Go Back
            </Button>
          </div>
        </div>
      </div>
    )
  }

  if (!renderInfo) {
    return (
      <div className="p-6">
        <div className="bg-white rounded-lg shadow p-6 text-center">
          <Text size="4" color="gray" weight="medium">
            Task not found.
          </Text>
          <div className="mt-4">
            <Button variant="soft" onClick={() => navigate(-1)}>
              <ArrowLeftIcon />
              Go Back
            </Button>
          </div>
        </div>
      </div>
    )
  }

  return (
    <div className="p-4 sm:p-6 lg:p-8 bg-gray-50 min-h-full">
      <div className="max-w-4xl mx-auto">
        <div className="mb-6">
          <Button variant="ghost" color="gray" onClick={() => navigate(-1)}>
            <ArrowLeftIcon />
            Back
          </Button>
        </div>

        <PluginRenderer response={renderInfo} />
      </div>
    </div>
  )
}