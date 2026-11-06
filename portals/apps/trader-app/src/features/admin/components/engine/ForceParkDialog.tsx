import { useState } from 'react'
import { Text, TextArea } from '@radix-ui/themes'
import { ConfirmDialog } from '@portals/ui'
import { useForcePark } from '@/features/admin/hooks'
import type { AdminWorkflowKind } from '@/features/admin/types'
import { IdentityField } from '@/features/admin/components/IdentityField'

// What force-parking does at each level, as the dialog explains it (see forceParkNode).
const FORCE_PARK_INFO: Record<AdminWorkflowKind, { description: string; next: string }> = {
  workflow: {
    description:
      "The node's whole task is closed, so the trader or officer cannot submit it, and the node parks for you to resolve. This can't be undone.",
    next: 'Then resolve it: RETRY with a variables patch to start a new task, COMPLETE with the outcome to move on, or ABORT to fail the workflow.',
  },
  task: {
    description:
      'Only this step parks; the task stays open. While it is parked, a submission for the step is rejected.',
    next: 'Then resolve it: COMPLETE with the variables the step would have produced lets the task carry on as if its callback had arrived; RETRY runs the step again; ABORT fails the task.',
  },
}

// Force-parks one RUNNING TASK node (see NodeRow's "Force park" button) after the admin gives a
// reason. It changes a live workflow with no undo, so this is a ConfirmDialog like the resolve
// confirmation. It stays open on an error so the admin can read it and try again.
export function ForceParkDialog({
  open,
  onOpenChange,
  workflowId,
  workflowKind,
  nodeId,
  stepId,
  onParked,
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
  workflowId: string
  workflowKind: AdminWorkflowKind
  nodeId: string
  stepId: string
  onParked: () => void
}) {
  const info = FORCE_PARK_INFO[workflowKind]
  const [reason, setReason] = useState('')
  const forcePark = useForcePark()
  const error = forcePark.error ? forcePark.error.message || 'Failed to force-park node.' : null

  const submit = async () => {
    try {
      await forcePark.mutate(workflowId, stepId, reason, workflowKind)
    } catch (err) {
      console.error('Failed to force-park node:', err)
      return
    }
    setReason('')
    onOpenChange(false)
    onParked()
  }

  return (
    <ConfirmDialog
      open={open}
      onOpenChange={onOpenChange}
      title={workflowKind === 'task' ? 'Force park step' : 'Force park node'}
      description={info.description}
      confirmLabel="Force park"
      cancelLabel="Cancel"
      onConfirm={() => void submit()}
      busy={forcePark.isPending}
      confirmDisabled={!reason.trim()}
      color="amber"
    >
      <div className="flex flex-wrap gap-4 mb-4 p-3 bg-app-surface-muted rounded">
        <IdentityField label="Workflow" id={workflowId} />
        <IdentityField label="Node" id={nodeId} />
      </div>

      <Text as="p" size="2" color="gray" className="mb-4">
        {info.next}
      </Text>

      <TextArea
        aria-label="Reason"
        value={reason}
        onChange={(e) => setReason(e.target.value)}
        rows={2}
        placeholder="Why are you force-parking this node? (required)"
      />

      {error && (
        <Text as="p" size="2" color="red" className="mt-3">
          {error}
        </Text>
      )}
    </ConfirmDialog>
  )
}
