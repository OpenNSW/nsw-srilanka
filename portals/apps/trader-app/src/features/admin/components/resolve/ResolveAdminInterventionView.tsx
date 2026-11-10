import { useMemo, useState } from 'react'
import { Button, Text, TextArea, Tooltip } from '@radix-ui/themes'
import { ConfirmDialog, StatusBadge, type StatusTone } from '@portals/ui'
import { ArrowLeftIcon, InfoCircledIcon } from '@radix-ui/react-icons'
import { useResolveAdminIntervention } from '@/features/admin/hooks'
import type { AdminResolutionAction, ParkCategory } from '@/features/admin/types'
import { IdentityField } from '@/features/admin/components/IdentityField'
import type { AdminResolutionTarget } from '@/features/admin/components/engine/shared'
import { LineNumberedTextArea } from '@/features/admin/components/resolve/LineNumberedTextArea'
import { MappingList } from '@/features/admin/components/resolve/MappingList'
import { Section } from '@/features/admin/components/resolve/Section'

// How each park_category reads on the resolve screen: a short label plus what an admin can usually
// do about it. The hints follow core's ParkCategory docs (workflow/park_category.go); they steer
// rather than prescribe, since the last error is still the ground truth.
const PARK_CATEGORY_INFO: Record<ParkCategory, { label: string; tone: StatusTone; hint: string }> = {
  INPUT_MAPPING: {
    label: 'Missing input',
    tone: 'warning',
    hint: "A variable this node reads isn't set. Set it in the variables and Retry.",
  },
  OUTPUT_MAPPING: {
    label: 'Missing output',
    tone: 'warning',
    hint: 'The task ran, but its result lacks a field the node maps. Complete sets the variables without running it again; Retry runs the task again.',
  },
  TASK_FAILURE: {
    label: 'Task failed',
    tone: 'warning',
    hint: "The node's own work failed. Retry runs it again; Complete moves past it.",
  },
  GATEWAY_CONDITION: {
    label: 'Gateway condition',
    tone: 'warning',
    hint: 'A routing condition failed to evaluate, or no outgoing edge matched. Fix a variable it reads and Retry.',
  },
  SPLIT_DATA: {
    label: 'Split data',
    tone: 'warning',
    hint: 'The items or branch data a split or join needs is missing or malformed. Fix the variable and Retry.',
  },
  CHILD_FAILURE: {
    label: 'Child workflow failed',
    tone: 'warning',
    hint: "A spawned child workflow failed. Open that child's own status to see why.",
  },
  DEFINITION_ERROR: {
    label: 'Definition error',
    tone: 'danger',
    hint: "The workflow definition itself is invalid, so setting variables won't fix it.",
  },
  ADMIN_FORCED: {
    label: 'Force-parked by admin',
    tone: 'warning',
    hint: 'An admin force-parked this waiting step. Complete with the outcome to move on, Retry to run it again, or Abort.',
  },
  UNKNOWN: {
    label: 'Unclassified',
    tone: 'neutral',
    hint: "This error wasn't categorized; the last error is the only guide.",
  },
}

// Which resolution actions exist and whether core's engine rejects them for a GATEWAY node (a
// gateway's routing can't be completed without bypassing its own condition logic — see
// core/workflow.parkNodeForAdmin). description is the chosen action's text under the buttons, and
// also a tooltip on an info button next to each action on wide screens — see the Action section
// below.
const ADMIN_ACTIONS: {
  action: AdminResolutionAction
  label: string
  color: 'blue' | 'green' | 'gray' | 'red'
  disabledForGateway: boolean
  description: string
}[] = [
  {
    action: 'RETRY',
    label: 'Retry',
    color: 'blue',
    disabledForGateway: false,
    description:
      "Re-runs the node for real — re-calls the Activity for a TASK node, or re-evaluates the routing condition for a GATEWAY. Any variables you set below are written first, so use it to fix a variable the node reads (e.g. one a GATEWAY's condition depends on) before it runs again.",
  },
  {
    action: 'COMPLETE',
    label: 'Complete',
    color: 'green',
    disabledForGateway: true,
    description:
      "Marks the node completed without running it (or running it again), then continues down its first outgoing edge. Any variables you set below are written first, standing in for the output the node would have produced; leave them empty to just move past the node. Unavailable for GATEWAY nodes: completing this way always takes the first outgoing edge, which would silently ignore a gateway's actual routing condition.",
  },
  {
    action: 'ABORT',
    label: 'Abort',
    color: 'red',
    disabledForGateway: false,
    description:
      "Fails this node and the whole workflow with the node's original error. Use when the workflow genuinely can't continue.",
  },
]

// Lets an admin resolve one AWAITING_ADMIN node (see NodeRow's "Resolve" button) by picking one
// of RETRY/COMPLETE/ABORT, a required reason, and an optional JSON variables patch. Takes over the
// whole screen (see EngineStatusView) rather than a dialog — last error, cached task result, and
// the patch JSON add up to more than a modal comfortably holds. Calls target.onResolved() on
// success so the caller can refresh just the affected instance's view, then onBack() — same as
// Back/Cancel, which both just return to the debugger view underneath without resolving anything.
export function ResolveAdminInterventionView({
  target,
  onBack,
}: {
  target: AdminResolutionTarget
  onBack: () => void
}) {
  // Falls back to UNKNOWN for a category this UI predates, rather than rendering nothing.
  const parkInfo = target.parkCategory
    ? (PARK_CATEGORY_INFO[target.parkCategory] ?? PARK_CATEGORY_INFO.UNKNOWN)
    : undefined
  const [action, setAction] = useState<AdminResolutionAction | null>(null)
  // What the patch editor is called: it is also the editor's accessible name, so it lives in one
  // place rather than being spelled out twice.
  const patchTitle =
    action === 'COMPLETE' ? "Set variables as this node's output (JSON)" : 'Set variables, then re-run (JSON)'
  const [reason, setReason] = useState('')
  // Starts empty rather than pre-filled from the cached task result: that result is in the
  // task's own key names, while the patch is written under global variable paths, so editing
  // it into shape invites writing the wrong key.
  const [variablesPatchText, setVariablesPatchText] = useState('{}')
  const resolve = useResolveAdminIntervention()
  const submitting = resolve.isPending
  // A patch that fails to parse is caught here, before anything is sent; a refused or failed
  // request is the mutation's error.
  const [patchError, setPatchError] = useState<string | null>(null)
  const error = patchError ?? (resolve.error ? resolve.error.message || 'Failed to resolve admin intervention.' : null)
  // Gates the actual submit behind an explicit re-confirmation — this mutates a live workflow
  // (possibly irreversibly for ABORT) and there's no undo, so "Submit" opens this dialog instead
  // of calling submit() directly.
  const [confirmOpen, setConfirmOpen] = useState(false)
  const currentAction = ADMIN_ACTIONS.find((a) => a.action === action)

  // Warns under the edit box while the draft isn't valid JSON — an invalid final value would
  // fail on submit.
  const variablesPatchJSONError = useMemo(() => {
    try {
      JSON.parse(variablesPatchText)
      return null
    } catch {
      return 'Not valid JSON yet.'
    }
  }, [variablesPatchText])

  const submit = async () => {
    if (!action) return
    setPatchError(null)
    // Only COMPLETE/RETRY actually consume the patch (see the ADMIN_ACTIONS descriptions above)
    // — for ABORT the editor isn't shown, so whatever it holds isn't anything the admin chose to
    // send and must not go out on the wire.
    let globalVariablesPatch: Record<string, unknown> | undefined
    if ((action === 'COMPLETE' || action === 'RETRY') && variablesPatchText.trim()) {
      try {
        globalVariablesPatch = JSON.parse(variablesPatchText) as Record<string, unknown>
      } catch {
        setConfirmOpen(false)
        setPatchError('Variables must be valid JSON.')
        return
      }
    }
    setConfirmOpen(false)
    try {
      await resolve.mutate(
        target.workflowId,
        target.stepId,
        { action, global_variables_patch: globalVariablesPatch, reason },
        target.workflowKind,
      )
    } catch (err) {
      console.error('Failed to resolve admin intervention:', err)
      return
    }
    target.onResolved()
    onBack()
  }

  return (
    <div className="p-4 md:p-6">
      <div className="mb-4">
        <Button variant="ghost" color="gray" onClick={onBack} disabled={submitting}>
          <ArrowLeftIcon />
          Back
        </Button>
      </div>

      <h1 className="text-xl font-semibold text-foreground mb-2">Resolve admin intervention</h1>
      <div className="flex flex-wrap gap-6 mb-6">
        <IdentityField label="Workflow" id={target.workflowId} />
        <IdentityField label="Node" id={target.nodeId} />
      </div>

      <div className="bg-app-surface rounded-lg shadow p-4 md:p-6 max-w-5xl">
        {parkInfo && (
          <Section title="Why it parked">
            <div className="flex items-center gap-2 flex-wrap">
              <StatusBadge tone={parkInfo.tone}>{parkInfo.label}</StatusBadge>
              <Text size="2" color="gray">
                {parkInfo.hint}
              </Text>
            </div>
          </Section>
        )}

        {target.lastError && (
          <Section title="Last error">
            <pre className="bg-app-surface-muted rounded p-3 text-xs font-mono overflow-auto max-h-32 whitespace-pre-wrap break-all">
              {target.lastError}
            </pre>
          </Section>
        )}

        {target.isGateway ? (
          <Section title="Outgoing conditions">
            {/* GATEWAY nodes have no CachedTaskResult (there's no Activity to run) — what an
                admin actually needs here is what each outgoing edge checks, since a "no matching
                conditions" park otherwise gives no way to know which variable to correct. Raw
                expr-lang text, not parsed — naming the variable(s) it references is enough to
                act on; evaluating it client-side isn't the point. */}
            {target.outgoingEdges && target.outgoingEdges.length > 0 ? (
              <div className="bg-app-surface-muted rounded p-3 text-xs font-mono overflow-auto max-h-48">
                {target.outgoingEdges.map((edge) => (
                  <div key={edge.id} className="whitespace-pre-wrap break-all py-0.5">
                    {edge.condition ? (
                      edge.condition
                    ) : (
                      <span className="text-foreground-muted">(default — no condition)</span>
                    )}
                  </div>
                ))}
              </div>
            ) : (
              <Text size="2" color="gray" className="block">
                No outgoing edges found for this node.
              </Text>
            )}
          </Section>
        ) : (
          <>
            <Section title="Cached task result">
              {target.cachedTaskResult ? (
                <pre className="bg-app-surface-muted rounded p-3 text-xs font-mono overflow-auto max-h-48 whitespace-pre-wrap break-all">
                  {JSON.stringify(target.cachedTaskResult, null, 2)}
                </pre>
              ) : (
                <Text size="2" color="gray" className="block">
                  No cached results for this node.
                </Text>
              )}
            </Section>
            {/* Which workflow variables a Retry reads and a Complete patch should write — see
                EngineNode.input_mapping / output_mapping for which side of each row is which. */}
            {target.inputMapping && Object.keys(target.inputMapping).length > 0 && (
              <MappingList
                title="Input mapping"
                note="Workflow variable → task input. Retry reads these variables, so set any that are missing."
                mapping={target.inputMapping}
              />
            )}
            {target.outputMapping && Object.keys(target.outputMapping).length > 0 && (
              <MappingList
                title="Output mapping"
                note="Task result field → workflow variable. A Complete patch should set the variables on the right."
                mapping={target.outputMapping}
              />
            )}
          </>
        )}

        {/* For every node, gateways included: a gateway parked on "no matching conditions" is fixed by
            finding which variable its conditions read is unset or wrong, and an input-mapping park by
            seeing which of the mapped variables exist. Same instance as the node (see
            AdminResolutionTarget.globalVariables), which is what a patch writes into. */}
        <Section
          title="Workflow variables"
          note="Current values in this workflow instance. A patch is written into these and stays for the rest of the workflow."
        >
          {target.globalVariables && Object.keys(target.globalVariables).length > 0 ? (
            <pre className="bg-app-surface-muted rounded p-3 text-xs font-mono overflow-auto max-h-64 whitespace-pre-wrap break-all">
              {JSON.stringify(target.globalVariables, null, 2)}
            </pre>
          ) : (
            <Text size="2" color="gray" className="block">
              No variables recorded for this workflow.
            </Text>
          )}
        </Section>

        <Section title="Action">
          <div className="flex gap-3 flex-wrap">
            {ADMIN_ACTIONS.map(({ action: candidate, label, color, disabledForGateway, description }) => {
              const disabled = target.isGateway && disabledForGateway
              return (
                <div key={candidate} className="flex items-center gap-1">
                  <Button
                    type="button"
                    variant={action === candidate ? 'solid' : 'soft'}
                    color={color}
                    size="2"
                    disabled={disabled}
                    aria-pressed={action === candidate}
                    title={disabled ? 'Not supported for GATEWAY nodes — use Retry or Abort' : undefined}
                    onClick={() => setAction(candidate)}
                  >
                    {label}
                  </Button>
                  {/* A button so the tooltip opens on keyboard focus as well as hover. A touch tap
                      doesn't open it, so the chosen action's description is also shown below at
                      every width; the tooltip is a shortcut for reading the others on wide screens. */}
                  <Tooltip content={description} maxWidth="320px">
                    <button
                      type="button"
                      aria-label={`What ${label} does`}
                      className="hidden md:inline-flex rounded text-foreground-muted cursor-help"
                    >
                      <InfoCircledIcon width={15} height={15} />
                    </button>
                  </Tooltip>
                </div>
              )
            })}
          </div>
          <Text as="p" size="2" color="gray" className="mt-4 text-justify" aria-live="polite">
            {currentAction?.description ?? 'Select an action to see what it does.'}
          </Text>
        </Section>

        <Section title="Reason (required)">
          <TextArea
            aria-label="Reason"
            value={reason}
            onChange={(e) => setReason(e.target.value)}
            rows={2}
            placeholder="Why are you resolving this node?"
          />
        </Section>

        {/* The patch only applies to COMPLETE and RETRY (ABORT never gets to signal anything) —
            so this only shows up once one of those is the chosen action. It is the same field
            either way — dotted paths written into the workflow's variables — and only the label
            differs by what happens next: RETRY writes them *before* re-running the node, so it
            reads them (e.g. through its input_mapping, or a GATEWAY's routing condition);
            COMPLETE writes them and skips running the node, so they stand in for its output. */}
        {(action === 'COMPLETE' || action === 'RETRY') && (
          <Section
            title={patchTitle}
            note={
              'Dotted paths to values, e.g. {"review.outcome": "APPROVED"}. Only the variables named are changed, ' +
              'and they stay changed for the rest of the workflow.'
            }
          >
            <LineNumberedTextArea value={variablesPatchText} onChange={setVariablesPatchText} label={patchTitle} />
            {variablesPatchJSONError && (
              <Text size="1" color="red" className="block mt-1">
                {variablesPatchJSONError}
              </Text>
            )}
          </Section>
        )}

        {error && (
          <Text size="2" color="red" className="block mb-2">
            {error}
          </Text>
        )}

        <div className="flex justify-end gap-2 mt-4">
          <Button variant="soft" color="gray" onClick={onBack} disabled={submitting}>
            Cancel
          </Button>
          <Button disabled={!action || !reason.trim() || submitting} onClick={() => setConfirmOpen(true)}>
            {submitting ? 'Submitting…' : 'Submit'}
          </Button>
        </div>

        {/* A live workflow signal, not a draft — there's no undo once it's sent (ABORT fails the
            whole workflow outright), so Submit opens this instead of calling submit() directly. */}
        <ConfirmDialog
          open={confirmOpen}
          onOpenChange={setConfirmOpen}
          title={`Confirm ${currentAction?.label ?? ''}`}
          description={
            <>
              You're about to <strong>{action}</strong> this node. This takes effect immediately and can't be undone.
            </>
          }
          confirmLabel={`Yes, ${currentAction?.label.toLowerCase() ?? ''}`}
          cancelLabel="Cancel"
          onConfirm={() => {
            setConfirmOpen(false)
            void submit()
          }}
          color={currentAction?.color}
        >
          {/* Kept free of the raw workflow/node IDs in the description, which made it a wall of
              text; they get their own compact block, the same layout as the identity block in the
              header above. */}
          <div className="flex flex-wrap gap-4 mb-4 p-3 bg-app-surface-muted rounded">
            <IdentityField label="Workflow" id={target.workflowId} />
            <IdentityField label="Node" id={target.nodeId} />
          </div>
          <Text as="p" size="2" color="gray">
            {currentAction?.description}
          </Text>
        </ConfirmDialog>
      </div>
    </div>
  )
}
