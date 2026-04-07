import { useState } from "react"
import { useParams } from "react-router-dom"
import { Button } from "@radix-ui/themes"
import { sendTaskAction } from "../services/task.ts"
import { JsonForms } from "@jsonforms/react"
import { radixRenderers } from "@opennsw/jsonforms-renderers"
import type {TaskFormData} from "./SimpleForm.tsx";

type WaitForEventDisplay = {
  title?: string
  description?: string
}

export type WaitForEventConfigs = {
  display?: WaitForEventDisplay
  eventReviewForm?: TaskFormData
}

// Shared radar/sonar animation used in both NOTIFIED_SERVICE and post-retry state.
function RadarIcon() {
  return (
    <div className="relative flex items-center justify-center w-28 h-28">
      <span className="absolute inline-flex w-28 h-28 rounded-full bg-indigo-100 animate-ping-slower opacity-40" />
      <span className="absolute inline-flex w-20 h-20 rounded-full bg-indigo-200 animate-ping-slow opacity-50" />
      <span className="relative inline-flex items-center justify-center w-14 h-14 rounded-full bg-indigo-600 shadow-lg shadow-indigo-200">
        {/* Signal / antenna icon */}
        <svg xmlns="http://www.w3.org/2000/svg" className="w-7 h-7 text-white" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={1.75}>
          <path strokeLinecap="round" strokeLinejoin="round" d="M8.288 15.038a5.25 5.25 0 017.424 0M5.106 11.856c3.807-3.808 9.98-3.808 13.788 0M1.924 8.674c5.565-5.565 14.587-5.565 20.152 0M12 20.25h.008v.008H12v-.008z" />
        </svg>
      </span>
    </div>
  )
}

function WaitingDots() {
  return (
    <div className="flex gap-1.5">
      <span className="w-2 h-2 rounded-full bg-indigo-400 animate-bounce" style={{ animationDelay: "0ms" }} />
      <span className="w-2 h-2 rounded-full bg-indigo-400 animate-bounce" style={{ animationDelay: "150ms" }} />
      <span className="w-2 h-2 rounded-full bg-indigo-400 animate-bounce" style={{ animationDelay: "300ms" }} />
    </div>
  )
}

// NOTIFIED_SERVICE — waiting for external callback
function WaitingState({ title, description, confirmedRetry }: {
  title: string
  description?: string
  confirmedRetry?: boolean
}) {
  return (
    <div className="bg-white rounded-xl shadow-md overflow-hidden animate-fade-in-up">
      <div className="h-1 bg-indigo-100 overflow-hidden relative">
        <div className="absolute inset-y-0 left-0 w-1/2 bg-indigo-500 animate-loading-bar" />
      </div>

      <div className="px-8 py-12 flex flex-col items-center text-center gap-4">
        <RadarIcon />

        <div className="space-y-1">
          <h2 className="text-xl font-bold text-gray-800">{title}</h2>
          {confirmedRetry && (
            <p className="text-sm font-medium text-indigo-600">Notification sent — waiting for response</p>
          )}
          {description && (
            <p className="text-sm text-gray-500 max-w-sm">{description}</p>
          )}
        </div>

        <WaitingDots />
      </div>
    </div>
  )
}

// NOTIFY_FAILED — notification to external service failed
function FailedState({ title, description, onRetry, isRetrying, retryError }: {
  title: string
  description?: string
  onRetry: () => void
  isRetrying: boolean
  retryError: string | null
}) {
  return (
    <div className="bg-white rounded-xl shadow-md overflow-hidden animate-fade-in">
      <div className="h-1 bg-amber-400" />

      <div className="px-8 py-12 flex flex-col items-center text-center gap-5">
        <div className="flex items-center justify-center w-16 h-16 rounded-full bg-amber-100">
          <svg xmlns="http://www.w3.org/2000/svg" className="w-8 h-8 text-amber-500" viewBox="0 0 20 20" fill="currentColor">
            <path fillRule="evenodd" d="M8.257 3.099c.765-1.36 2.722-1.36 3.486 0l5.58 9.92c.75 1.334-.213 2.98-1.742 2.98H4.42c-1.53 0-2.493-1.646-1.743-2.98l5.58-9.92zM11 13a1 1 0 11-2 0 1 1 0 012 0zm-1-8a1 1 0 00-1 1v3a1 1 0 002 0V6a1 1 0 00-1-1z" clipRule="evenodd" />
          </svg>
        </div>

        <div className="space-y-1">
          <h2 className="text-xl font-bold text-gray-800">{title}</h2>
          <p className="text-sm text-gray-500">Could not reach the external service. You can retry below.</p>
          {description && (
            <p className="text-sm text-gray-400 max-w-sm">{description}</p>
          )}
        </div>

        <Button onClick={onRetry} disabled={isRetrying} size="3">
          {isRetrying ? (
            <span className="flex items-center gap-2">
              <svg className="animate-spin w-4 h-4" xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24">
                <circle className="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="4" />
                <path className="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8v8z" />
              </svg>
              Retrying...
            </span>
          ) : (
            <span className="flex items-center gap-2">
              <svg xmlns="http://www.w3.org/2000/svg" className="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
                <path strokeLinecap="round" strokeLinejoin="round" d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15" />
              </svg>
              Retry
            </span>
          )}
        </Button>

        {retryError && (
          <div className="w-full bg-red-50 border border-red-200 text-red-700 text-sm rounded-lg px-4 py-3">
            {retryError}
          </div>
        )}
      </div>
    </div>
  )
}

function ReviewResponseForm({ formInfo }: { formInfo: TaskFormData }) {
  return (
    <div className="bg-white rounded-xl shadow-md overflow-hidden mt-6 animate-fade-in-up">
      <div className="bg-gray-50 border-b border-gray-100 px-6 py-4">
        <h3 className="text-sm font-bold text-gray-700 uppercase tracking-wider">{formInfo.title}</h3>
      </div>
      <div className="p-6 pointer-events-none opacity-80 bg-gray-50/30">
        <JsonForms
          schema={formInfo.schema}
          uischema={formInfo.uiSchema}
          data={formInfo.formData}
          renderers={radixRenderers}
          readonly={true}
        />
      </div>
    </div>
  )
}

// RECEIVED_CALLBACK — external service has responded
function CompletedState({ title, description, formInfo }: { 
  title: string; 
  description?: string;
  formInfo?: TaskFormData
}) {
  return (
    <div className="space-y-6">
      <div className="bg-white rounded-xl shadow-md overflow-hidden animate-fade-in-up">
        <div className="h-1 bg-emerald-500" />

        <div className="px-8 py-12 flex flex-col items-center text-center gap-4">
          <div className="flex items-center justify-center w-16 h-16 rounded-full bg-emerald-100 animate-scale-pulse">
            <svg xmlns="http://www.w3.org/2000/svg" className="w-8 h-8 text-emerald-600" viewBox="0 0 20 20" fill="currentColor">
              <path fillRule="evenodd" d="M16.707 5.293a1 1 0 010 1.414l-8 8a1 1 0 01-1.414 0l-4-4a1 1 0 011.414-1.414L8 12.586l7.293-7.293a1 1 0 011.414 0z" clipRule="evenodd" />
            </svg>
          </div>

          <div className="space-y-1">
            <p className="text-xs font-semibold uppercase tracking-widest text-emerald-600">Complete</p>
            <h2 className="text-xl font-bold text-gray-800">{title}</h2>
            {description && (
              <p className="text-sm text-gray-500 max-w-sm">{description}</p>
            )}
          </div>
        </div>
      </div>

      {formInfo && <ReviewResponseForm formInfo={formInfo} />}
    </div>
  )
}

export default function WaitForEvent(props: { configs: WaitForEventConfigs; pluginState: string }) {
  const { consignmentId, preConsignmentId, taskId } = useParams<{
    consignmentId?: string
    preConsignmentId?: string
    taskId?: string
  }>()
  const [isRetrying, setIsRetrying] = useState(false)
  const [retryError, setRetryError] = useState<string | null>(null)
  const [retried, setRetried] = useState(false)

  const workflowId = preConsignmentId || consignmentId
  const display = props.configs.display
  const title = display?.title ?? "Waiting for event"
  const description = display?.description

  const handleRetry = async () => {
    if (!workflowId || !taskId) return
    setIsRetrying(true)
    setRetryError(null)
    try {
      const response = await sendTaskAction(taskId, workflowId, "RETRY")
      if (response.success) {
        setRetried(true)
      } else {
        setRetryError(response.error?.message ?? "Retry failed. Please try again.")
      }
    } catch (err) {
      console.error("Error retrying:", err)
      setRetryError("Retry failed. Please try again.")
    } finally {
      setIsRetrying(false)
    }
  }

  if (props.pluginState === "RECEIVED_CALLBACK") {
    return <CompletedState title={title} description={description} formInfo={props.configs.eventReviewForm} />
  }

  if (props.pluginState === "NOTIFY_FAILED" && !retried) {
    return (
      <FailedState
        title={title}
        description={description}
        onRetry={handleRetry}
        isRetrying={isRetrying}
        retryError={retryError}
      />
    )
  }

  return <WaitingState title={title} description={description} confirmedRetry={retried} />
}
