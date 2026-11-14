import { Button, Dialog, Text } from '@radix-ui/themes'
import { CodeBlock } from '@portals/ui'
import type { WorkflowVariablesTarget } from '@/features/admin/components/engine/shared'

// Global variables belong to a workflow instance (root or child), not to any one node — opened
// from a "Global variables" action per workflow instance rather than per node row, which would
// misleadingly imply the data is node-specific.
export function WorkflowVariablesDialog({
  target,
  onClose,
}: {
  target: WorkflowVariablesTarget | null
  onClose: () => void
}) {
  return (
    <Dialog.Root open={target !== null} onOpenChange={(open) => !open && onClose()}>
      <Dialog.Content maxWidth="600px">
        {target && (
          <>
            <Dialog.Title>{target.label} global variables</Dialog.Title>
            <Dialog.Description size="2" color="gray" className="font-mono break-all mb-4">
              {target.workflowId}
            </Dialog.Description>

            {target.variables && Object.keys(target.variables).length > 0 ? (
              <CodeBlock label={`${target.label} global variables`} className="max-h-96">
                {JSON.stringify(target.variables, null, 2)}
              </CodeBlock>
            ) : (
              <Text size="2" color="gray">
                No global variables recorded for this workflow.
              </Text>
            )}

            <div className="flex justify-end mt-4">
              <Dialog.Close>
                <Button variant="soft" color="gray">
                  Close
                </Button>
              </Dialog.Close>
            </div>
          </>
        )}
      </Dialog.Content>
    </Dialog.Root>
  )
}
