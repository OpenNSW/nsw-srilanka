import { useEffect, useState } from 'react'
import { useParams, useNavigate } from 'react-router-dom'
import { Button, Badge, Spinner, Text } from '@radix-ui/themes'
import { ArrowLeftIcon } from '@radix-ui/react-icons'
import { WorkflowViewer } from '../components/WorkflowViewer'
import type { StepStatuses } from '../components/WorkflowViewer'
import type {Consignment} from "../services/types/consignment.ts";
import type {WorkflowDefinition} from "../services/types/workflow.ts";
import {getConsignment} from "../services/consignment.ts";
import {getWorkflowById} from "../services/workflow.ts";

function getStatusColor(status: Consignment['status']): 'gray' | 'blue' | 'orange' | 'green' | 'red' {
  switch (status) {
    case 'draft':
      return 'gray'
    case 'pending':
      return 'blue'
    case 'in_progress':
      return 'orange'
    case 'completed':
      return 'green'
    case 'rejected':
      return 'red'
    default:
      return 'gray'
  }
}

function formatStatus(status: Consignment['status']): string {
  return status.replace('_', ' ').replace(/\b\w/g, (c) => c.toUpperCase())
}

// Mock step statuses for demonstration
// In a real app, this would come from the backend based on actual execution state
function getMockStepStatuses(definition: WorkflowDefinition): StepStatuses {
  const statuses: StepStatuses = {}
  const completedSteps = new Set<string>()

  // For demo: mark first step (cusdec) as COMPLETED
  const firstStep = definition.steps.find(s => s.dependsOn.length === 0)
  if (firstStep) {
    statuses[firstStep.id] = 'COMPLETED'
    completedSteps.add(firstStep.id)
  }

  // Process remaining steps
  definition.steps.forEach(step => {
    if (completedSteps.has(step.id)) return

    // Check if all dependencies are completed
    const allDepsCompleted = step.dependsOn.every(depId => completedSteps.has(depId))

    if (allDepsCompleted) {
      // Step is ready to execute - mark as ACTIVE
      statuses[step.id] = 'ACTIVE'
    } else {
      // Step is waiting for dependencies - mark as LOCKED
      statuses[step.id] = 'LOCKED'
    }
  })

  return statuses
}

export function ConsignmentDetailScreen() {
  const { consignmentId } = useParams<{ consignmentId: string }>()
  const navigate = useNavigate()
  const [consignment, setConsignment] = useState<Consignment | null>(null)
  const [workflowDefinition, setWorkflowDefinition] = useState<WorkflowDefinition | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    async function fetchConsignmentAndWorkflow() {
      if (!consignmentId) {
        setError('Consignment ID is required')
        setLoading(false)
        return
      }

      try {
        const result = await getConsignment(consignmentId)
        if (result) {
          setConsignment(result)

          // Fetch workflow to get the diagram
          try {
            const workflow = await getWorkflowById(result.workflowId)
            if (workflow?.definition) {
              setWorkflowDefinition(workflow.definition)
            }
          } catch (wfError) {
            console.error('Failed to fetch workflow:', wfError)
            // We don't fail the whole page if workflow diagram fails
          }
        } else {
          setError('Consignment not found')
        }
      } catch (err) {
        console.error('Failed to fetch consignment:', err)
        setError('Failed to load consignment')
      } finally {
        setLoading(false)
      }
    }

    fetchConsignmentAndWorkflow()
  }, [consignmentId])

  if (loading) {
    return (
      <div className="p-6">
        <div className="flex items-center justify-center py-12">
          <Spinner size="3" />
          <Text size="3" color="gray" className="ml-3">
            Loading consignment...
          </Text>
        </div>
      </div>
    )
  }

  if (error || !consignment) {
    return (
      <div className="p-6">
        <div className="bg-white rounded-lg shadow p-6 text-center">
          <Text size="4" color="red" weight="medium">
            {error || 'Consignment not found'}
          </Text>
          <div className="mt-4">
            <Button variant="soft" onClick={() => navigate('/')}>
              <ArrowLeftIcon />
              Back to Dashboard
            </Button>
          </div>
        </div>
      </div>
    )
  }

  return (
    <div className="p-6">
      <div className="mb-6">
        <Button variant="ghost" color="gray" onClick={() => navigate(-1)}>
          <ArrowLeftIcon />
          Back to Dashboard
        </Button>
      </div>

      <div className="bg-white rounded-lg shadow">
        <div className="p-6 border-b border-gray-200">
          <div className="flex items-center justify-between">
            <div>
              <h1 className="text-2xl font-semibold text-gray-900">
                Consignment {consignment.id}
              </h1>
              <p className="mt-1 text-sm text-gray-500">
                Created on {new Date(consignment.createdAt).toLocaleDateString('en-US', {
                  year: 'numeric',
                  month: 'long',
                  day: 'numeric',
                  hour: '2-digit',
                  minute: '2-digit',
                })}
              </p>
            </div>
            <Badge size="2" color={getStatusColor(consignment.status)}>
              {formatStatus(consignment.status)}
            </Badge>
          </div>
        </div>

        <div className="p-6">
          <div className="grid grid-cols-1 md:grid-cols-2 gap-6">
            <div>
              <h3 className="text-sm font-medium text-gray-500 mb-4">HS Code Details</h3>
              <div className="space-y-3">
                <div>
                  <p className="text-xs text-gray-400">Code</p>
                  <p className="text-sm font-medium text-gray-900">{consignment.hsCode}</p>
                </div>
                <div>
                  <p className="text-xs text-gray-400">Description</p>
                  <p className="text-sm text-gray-900">{consignment.hsCodeDescription}</p>
                </div>
              </div>
            </div>

            <div>
              <h3 className="text-sm font-medium text-gray-500 mb-4">Workflow Details</h3>
              <div className="space-y-3">
                <div>
                  <p className="text-xs text-gray-400">Workflow Name</p>
                  <p className="text-sm font-medium text-gray-900">{consignment.workflowName}</p>
                </div>
                <div>
                  <p className="text-xs text-gray-400">Type</p>
                  <Badge size="1" color={consignment.workflowType === 'import' ? 'blue' : 'green'}>
                    {consignment.workflowType.charAt(0).toUpperCase() + consignment.workflowType.slice(1)}
                  </Badge>
                </div>
              </div>
            </div>
          </div>
        </div>

        {workflowDefinition && (
          <div className="p-6 border-t border-gray-200">
            <h3 className="text-sm font-medium text-gray-500 mb-4">Workflow Process</h3>
            <WorkflowViewer
              definition={workflowDefinition}
              stepStatuses={getMockStepStatuses(workflowDefinition)}
            />
          </div>
        )}

        <div className="p-6 border-t border-gray-200 bg-gray-50">
          <h3 className="text-sm font-medium text-gray-500 mb-4">Next Steps</h3>
          <p className="text-sm text-gray-600">
            Your consignment has been created and is pending review. You will be notified once it has been processed.
          </p>
        </div>
      </div>
    </div>
  )
}