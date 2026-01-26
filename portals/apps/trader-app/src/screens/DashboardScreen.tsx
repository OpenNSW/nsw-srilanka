import { useState, useEffect } from 'react'
import { useNavigate } from 'react-router-dom'
import { Button, AlertDialog, Box, Flex, Text } from '@radix-ui/themes'
import { HSCodePicker } from '../components/HSCodePicker'
import type {HSCode} from "../services/types/hsCode.ts";
import type {Workflow} from "../services/types/workflow.ts";
import type {Consignment} from "../services/types/consignment.ts";
import {createConsignment, getAllConsignments} from "../services/consignment.ts";

export function DashboardScreen() {
  const navigate = useNavigate()
  const [pickerOpen, setPickerOpen] = useState(false)
  const [creating, setCreating] = useState(false)
  const [pendingSelection, setPendingSelection] = useState<{ hsCode: HSCode; workflow: Workflow } | null>(null)

  const [consignments, setConsignments] = useState<Consignment[]>([])
  const [loading, setLoading] = useState(true)

  useEffect(() => {
    async function fetchConsignments() {
      try {
        const data = await getAllConsignments()
        setConsignments(data)
      } catch (error) {
        console.error('Failed to fetch consignments:', error)
      } finally {
        setLoading(false)
      }
    }

    fetchConsignments()
  }, [])

  const handleSelect = (hsCode: HSCode, workflow: Workflow) => {
    setPickerOpen(false)
    // Show confirmation dialog
    setPendingSelection({ hsCode, workflow })
  }

  const handleConfirmCreate = async () => {
    if (!pendingSelection) return

    const { hsCode, workflow } = pendingSelection
    setCreating(true)
    setPendingSelection(null)

    try {
      const response = await createConsignment({
        hsCode: hsCode.code,
        hsCodeDescription: hsCode.description,
        workflowId: workflow.id,
        workflowName: workflow.name,
        workflowType: workflow.type,
      })

      // Navigate to the consignment detail page
      navigate(`/consignments/${response.consignmentId}`)
    } catch (error) {
      console.error('Failed to create consignment:', error)
      // Could show an error toast here
    } finally {
      setCreating(false)
    }
  }

  const totalConsignments = consignments.length
  const inProgressConsignments = consignments.filter(c => c.status === 'in_progress').length
  const completedConsignments = consignments.filter(c => c.status === 'completed').length

  return (
    <div className="p-6">
      <div className="flex items-center justify-between mb-6">
        <h1 className="text-2xl font-semibold text-gray-900">Dashboard</h1>
        <Button onClick={() => setPickerOpen(true)} disabled={creating}>
          {creating ? 'Creating...' : 'New Consignment'}
        </Button>
      </div>
      <div className="grid grid-cols-1 md:grid-cols-3 gap-6">
        <div className="bg-white rounded-lg shadow p-6">
          <h3 className="text-sm font-medium text-gray-500">Total Consignments</h3>
          <p className="mt-2 text-3xl font-semibold text-gray-900">{loading ? '-' : totalConsignments}</p>
        </div>
        <div className="bg-white rounded-lg shadow p-6">
          <h3 className="text-sm font-medium text-gray-500">In Progress</h3>
          <p className="mt-2 text-3xl font-semibold text-gray-900">{loading ? '-' : inProgressConsignments}</p>
        </div>
        <div className="bg-white rounded-lg shadow p-6">
          <h3 className="text-sm font-medium text-gray-500">Completed</h3>
          <p className="mt-2 text-3xl font-semibold text-gray-900">{loading ? '-' : completedConsignments}</p>
        </div>
      </div>

      <HSCodePicker
        open={pickerOpen}
        onOpenChange={setPickerOpen}
        onSelect={handleSelect}
        title="New Consignment"
        description="Select an HS Code and workflow to create a new consignment."
        confirmText="Continue"
      />

      {/* Confirmation Dialog */}
      <AlertDialog.Root open={!!pendingSelection} onOpenChange={(open) => !open && setPendingSelection(null)}>
        <AlertDialog.Content maxWidth="450px">
          <AlertDialog.Title>Start Consignment</AlertDialog.Title>
          <AlertDialog.Description size="2">
            Are you sure you want to start a new consignment with the following details?
          </AlertDialog.Description>

          {pendingSelection && (
            <Box mt="4" p="3" className="bg-gray-50 rounded-md">
              <Flex direction="column" gap="2">
                <Flex justify="between">
                  <Text size="2" color="gray">HS Code:</Text>
                  <Text size="2" weight="medium">{pendingSelection.hsCode.code}</Text>
                </Flex>
                <Flex justify="between">
                  <Text size="2" color="gray">Description:</Text>
                  <Text size="2" weight="medium" style={{ textAlign: 'right', maxWidth: '250px' }}>
                    {pendingSelection.hsCode.description}
                  </Text>
                </Flex>
                <Flex justify="between">
                  <Text size="2" color="gray">Workflow:</Text>
                  <Text size="2" weight="medium">{pendingSelection.workflow.name}</Text>
                </Flex>
                <Flex justify="between">
                  <Text size="2" color="gray">Type:</Text>
                  <Text size="2" weight="medium" style={{ textTransform: 'capitalize' }}>
                    {pendingSelection.workflow.type}
                  </Text>
                </Flex>
              </Flex>
            </Box>
          )}

          <Flex gap="3" mt="4" justify="end">
            <AlertDialog.Cancel>
              <Button variant="soft" color="gray">
                Cancel
              </Button>
            </AlertDialog.Cancel>
            <AlertDialog.Action>
              <Button onClick={handleConfirmCreate}>
                Start Consignment
              </Button>
            </AlertDialog.Action>
          </Flex>
        </AlertDialog.Content>
      </AlertDialog.Root>
    </div>
  )
}