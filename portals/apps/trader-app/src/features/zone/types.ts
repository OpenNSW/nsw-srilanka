import type { JsonSchema, UISchemaElement } from '@jsonforms/core'
import type { TFunction } from 'i18next'
import type { ReactNode } from 'react'

export type FormPayload = {
  schema: JsonSchema
  uiSchema?: UISchemaElement
  data?: Record<string, unknown>
  readonly?: boolean
}

export type MarkdownPayload = {
  content: string
}

export type RedirectPayload = {
  checkout_url: string
  content: string
}

export type AlertVariant = 'info' | 'success' | 'warning' | 'error'

export type Alert = string | { message: string; title?: string; variant?: AlertVariant }

// Handle is one operation as the trader-app receives it: command identifies
// what to dispatch, label is the user-facing text, element is an identifier
// owned by this zone's renderer (e.g. 'primary_action', 'secondary_action'
// for a FORM zone). i18n holds label translations keyed by language
// code (e.g. 'si', 'ta'); see handleLabel. Dispatch behavior — whether to
// gather form data, validation gating — is decided by the renderer based on
// element, not by any field on the handle itself.
export type Handle = {
  command: string
  label: string
  i18n?: Record<string, string>
  element?: string
}

export const HANDLE_LABEL_KEYS = {
  'Save as Draft': 'tasks.handles.saveAsDraft',
  Continue: 'tasks.handles.continue',
  'Continue >': 'tasks.handles.continue',
  'Save & Continue': 'tasks.handles.saveAndContinue',
  'Complete Selection': 'tasks.handles.completeSelection',
  'Proceed to Payment': 'tasks.handles.proceedToPayment',
  'Submit Application': 'tasks.handles.submitApplication',
  'Submit application': 'tasks.handles.submitApplication',
  'Submit Decision': 'tasks.handles.submitDecision',
  'Submit decision': 'tasks.handles.submitDecision',
  Done: 'tasks.handles.done',
  'Upload Documents': 'tasks.handles.uploadDocuments',
  'Submit Documents': 'tasks.handles.submitDocuments',
  Submit: 'tasks.handles.submit',
  'Schedule Inspection': 'tasks.handles.scheduleInspection',
  'Acknowledge Amendment': 'tasks.handles.acknowledgeAmendment',
  Consolidate: 'tasks.handles.consolidate',
  'Look Again': 'tasks.handles.lookAgain',
  Verify: 'tasks.handles.verify',
  'Submit Blend Sheet': 'tasks.handles.submitBlendSheet',
  'Submit Boat Note': 'tasks.handles.submitBoatNote',
  'Submit CDN Application': 'tasks.handles.submitCDNApplication',
  'Submit Certificates': 'tasks.handles.submitCertificates',
  'Submit CusDec Application': 'tasks.handles.submitCusDecApplication',
  'Submit Inspection Schedule': 'tasks.handles.submitInspectionSchedule',
  'Submit Payment Slip': 'tasks.handles.submitPaymentSlip',
  'Submit Receipt': 'tasks.handles.submitReceipt',
  'Submit Response': 'tasks.handles.submitResponse',
  'Submit Treatment Request': 'tasks.handles.submitTreatmentRequest',
  'Submit inspection outcome': 'tasks.handles.submitInspectionOutcome',
  'Submit to the Export Division': 'tasks.handles.submitToExportDivision',
  'Fetch Pre-advised Containers': 'tasks.handles.fetchPreAdvisedContainers',
  'Generate Invoice': 'tasks.handles.generateInvoice',
  'Raise Service Order': 'tasks.handles.raiseServiceOrder',
  'Raise Sundry Order': 'tasks.handles.raiseSundryOrder',
} as const

// handleLabel resolves the localized label for a handle. If an explicit
// translation is provided in handle.i18n, it is prioritized; otherwise it
// falls back to the frontend translation dictionary before returning handle.label.
export function handleLabel(handle: Handle, language?: string, t?: TFunction): string {
  if (language && handle.i18n?.[language]) {
    return handle.i18n[language]
  }
  if (t) {
    const trimmed = handle.label.trim()
    if (trimmed in HANDLE_LABEL_KEYS) {
      const key = HANDLE_LABEL_KEYS[trimmed as keyof typeof HANDLE_LABEL_KEYS]
      const translated = t(key)
      if (translated) {
        return String(translated)
      }
    }
    if (handle.element === 'draft_action') {
      const draftText = t('tasks.handles.saveAsDraft')
      if (draftText) {
        return String(draftText)
      }
    }
  }
  return handle.label
}

// ZoneAction dispatches a handle a zone's renderer activated, with the data it
// gathered. draft is set when the renderer's element saves a draft of the step
// rather than completing it (a FORM zone's 'draft_action'); the caller then
// stores the data without moving the task on, and command is not sent.
export type ZoneAction = (
  command: string,
  data: Record<string, unknown>,
  options?: { draft?: boolean },
) => Promise<void>

// id is the section key from the task's render config. unique within a view.
type ZoneComponentBase = {
  id: string
  title?: string
  handles?: Handle[]
}

export type ZoneComponent =
  | (ZoneComponentBase & { type: 'FORM'; payload: FormPayload })
  | (ZoneComponentBase & { type: 'MARKDOWN'; payload: MarkdownPayload })
  | (ZoneComponentBase & { type: 'REDIRECT'; payload: RedirectPayload })

export type AuditEntry = {
  timestamp: string
  actor: string
  event: string
  from_state?: string
  to_state?: string
  details?: string
}

// ClaimHolder is the user holding a task's claim. name is their email for now, and
// empty if it could not be found. It carries no user id: use TaskClaim.mine.
export type ClaimHolder = {
  name: string
}

// TaskClaim is who holds the task's claim, as the caller sees it. mine is decided
// by the server: never compare ids or emails here.
export type TaskClaim = {
  holder?: ClaimHolder
  claimed_at?: string
  mine: boolean
}

// ZoneView is the wire shape served by GET /api/v1/tasks/{id}. There is
// no separate top-level actions list — operations ship inside their claiming
// zone's handles (joined to state legality by the backend assembler).
export type ZoneView = {
  task_id: string
  task_type: string
  state: string
  // step_id is the step the task is on. A submission is posted to it, so one made
  // against a step the task has since left is rejected (409) instead of applied.
  step_id?: string
  alert?: Alert
  audit?: AuditEntry[]
  // claim is absent where the deployment does not use task claims.
  claim?: TaskClaim
  view: ZoneComponent[]
  created_at: string
  updated_at: string
}

export type ZoneRendererProps<T extends ZoneComponent['type']> = {
  payload: Extract<ZoneComponent, { type: T }>['payload']
}

export type ZoneRenderer<T extends ZoneComponent['type']> = (props: ZoneRendererProps<T>) => ReactNode
