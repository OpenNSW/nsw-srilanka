import { useState, useEffect } from 'react'
import { useParams, useNavigate } from 'react-router-dom'
import { Button, Spinner, Text } from '@radix-ui/themes'
import { ArrowLeftIcon } from '@radix-ui/react-icons'
import { JsonForm } from '../components/JsonForm'
import type { JsonSchema, Layout } from '../components/JsonForm'
import { executeTask } from '../services/workflow'
import type { TaskDetails } from '../services/mocks/taskData'

interface FormPayload {
  version: number
  content: {
    schema: JsonSchema
    uischema: Layout
  }
}

export function FormScreen() {
  const { consignmentId, taskId } = useParams<{
    consignmentId: string
    taskId: string
  }>()
  const navigate = useNavigate()
  const [task, setTask] = useState<TaskDetails | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    async function fetchTask() {
      if (!consignmentId || !taskId) {
        setError('Consignment ID or Task ID is missing.')
        setLoading(false)
        return
      }

      try {
        setLoading(true)
        const taskDetails = await executeTask(consignmentId, taskId)
        setTask(taskDetails)
      } catch (err) {
        setError('Failed to fetch task details.')
        console.error(err)
      } finally {
        setLoading(false)
      }
    }

    fetchTask()
  }, [consignmentId, taskId])

  const handleSubmit = (formData: unknown) => {
    console.log('Form submitted!', formData)
    // Here you would typically call an API to save the data
    alert('Form submitted successfully!')
    navigate(`/consignments/${consignmentId}`)
  }

  if (loading) {
    return (
      <div className="flex justify-center items-center h-full p-6">
        <Spinner size="3" />
        <Text size="3" color="gray" className="ml-3">
          Loading form...
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

  if (!task) {
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

  const payload = task.payload as FormPayload

  return (
    <div className="p-4 sm:p-6 lg:p-8 bg-gray-50 min-h-full">
      <div className="max-w-4xl mx-auto">
        <div className="mb-6">
          <Button variant="ghost" color="gray" onClick={() => navigate(-1)}>
            <ArrowLeftIcon />
            Back
          </Button>
        </div>

        <div className="bg-white rounded-lg shadow-md p-6 mb-6">
          <h1 className="text-2xl font-bold text-gray-800">{task.name}</h1>
          <p className="text-gray-600 mt-2">{task.description}</p>
        </div>

        <div className="bg-white rounded-lg shadow-md p-6">
          <JsonForm
            schema={payload.content.schema}
            uischema={payload.content.uischema}
            onSubmit={handleSubmit}
            submitLabel="Submit Form"
          />
        </div>
      </div>
    </div>
  )
}