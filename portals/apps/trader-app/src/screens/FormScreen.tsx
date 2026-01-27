import { useState, useEffect } from 'react'
import { useParams, useNavigate } from 'react-router-dom'
import { Button, Spinner, Text } from '@radix-ui/themes'
import { ArrowLeftIcon } from '@radix-ui/react-icons'
import { JsonForm } from '../components/JsonForm'
import { executeTask, sendTaskCommand } from '../services/task'
import type { TaskFormData } from '../services/task'

export function FormScreen() {
  const { consignmentId, taskId } = useParams<{
    consignmentId: string
    taskId: string
  }>()
  const navigate = useNavigate()
  const [formData, setFormData] = useState<TaskFormData | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    async function fetchForm() {
      if (!consignmentId || !taskId) {
        setError('Consignment ID or Task ID is missing.')
        setLoading(false)
        return
      }

      try {
        setLoading(true)
        // Execute task with FETCH_FORM action to get the form schema
        const response = await executeTask(consignmentId, taskId, 'SIMPLE_FORM')

        if (response.success && response.result.data) {
          setFormData(response.result.data)
        } else {
          setError(response.result?.message || 'Failed to fetch form.')
        }
      } catch (err) {
        setError('Failed to fetch form details.')
        console.error(err)
      } finally {
        setLoading(false)
      }
    }

    fetchForm()
  }, [consignmentId, taskId])

  const handleSubmit = async (data: unknown) => {
    if (!consignmentId || !taskId) {
      setError('Consignment ID or Task ID is missing.')
      return
    }

    try {
      setError(null)

      // Send form submission with SUBMIT_FORM action
      const response = await sendTaskCommand({
        command: 'SUBMISSION',
        taskId,
        consignmentId,
        data: data as Record<string, unknown>,
      })

      if (response.success) {
        console.log('Form submitted successfully:', response)
        // Navigate back to consignment details with a flag to trigger delayed refresh
        navigate(`/consignments/${consignmentId}`, { 
          state: { justSubmitted: true } 
        })
      } else {
        setError(response.message || 'Failed to submit form.')
      }
    } catch (err) {
      console.error('Error submitting form:', err)
      setError('Failed to submit form. Please try again.')
    } finally {
    }
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

  if (!formData) {
    return (
      <div className="p-6">
        <div className="bg-white rounded-lg shadow p-6 text-center">
          <Text size="4" color="gray" weight="medium">
            Form not found.
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

        <div className="bg-white rounded-lg shadow-md p-6 mb-6">
          <h1 className="text-2xl font-bold text-gray-800">{formData.title}</h1>
        </div>

        <div className="bg-white rounded-lg shadow-md p-6">
          {<JsonForm
            schema={formData.schema}
            uiSchema={formData.uiSchema}
            data={formData.formData}
            onSubmit={handleSubmit}
            submitLabel="Submit Form"
          />}
        </div>
      </div>
    </div>
  )
}