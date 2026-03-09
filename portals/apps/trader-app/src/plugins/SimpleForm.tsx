import { JsonForms } from '@jsonforms/react';
import { radixRenderers } from '@opennsw/jsonforms-renderers';
import { sendTaskCommand } from "../services/task.ts";
import { useApi } from '../services/ApiContext'

import { useLocation, useNavigate, useParams } from "react-router-dom";
import { useState, useCallback } from "react";
import { Button } from "@radix-ui/themes";
import type { JsonSchema, UISchemaElement } from '@jsonforms/core';
import { autoFillForm } from "../utils/formUtils";
import { getBooleanEnv } from '../runtimeConfig';



export interface TaskFormData {
  title: string
  schema: JsonSchema
  uiSchema: UISchemaElement
  formData: Record<string, unknown>
}

export interface OGAFeedbackEntry {
  content: Record<string, unknown>
  timestamp: string
  round: number
}

export type SimpleFormConfig = {
  traderFormInfo: TaskFormData
  ogaReviewForm?: TaskFormData
  submissionResponseForm?: TaskFormData
  ogaFeedback?: OGAFeedbackEntry[]
}

function TraderForm(props: { formInfo: TaskFormData, pluginState: string }) {
  const { consignmentId, preConsignmentId, taskId } = useParams<{
    consignmentId?: string
    preConsignmentId?: string
    taskId?: string
  }>()
  const location = useLocation()
  const navigate = useNavigate()
  const api = useApi()
  const [data, setData] = useState<Record<string, unknown>>(props.formInfo.formData || {})
  const [errors, setErrors] = useState<any[]>([])
  const [submitError, setSubmitError] = useState<string | null>(null)
  const [isSubmitting, setIsSubmitting] = useState(false)

  const READ_ONLY_STATES = ['OGA_REVIEWED', 'SUBMITTED', 'OGA_ACKNOWLEDGED'];
  const isReadOnly = READ_ONLY_STATES.includes(props.pluginState);

  const isPreConsignment = location.pathname.includes('/pre-consignments/')
  const workflowId = preConsignmentId || consignmentId



  const handleFormAction = async (command: 'SUBMISSION' | 'SAVE_AS_DRAFT') => {
    if (!workflowId || !taskId) {
      setSubmitError('Workflow ID or Task ID is missing.');
      return;
    }

    if (command === 'SUBMISSION' && errors.length > 0) {
      setSubmitError('Please fix validation errors before submitting.');
      return;
    }

    setIsSubmitting(true);
    setSubmitError(null);

    const isDraft = command === 'SAVE_AS_DRAFT';
    const actionText = isDraft ? 'save draft' : 'submit form';
    const consoleActionText = isDraft ? 'saving draft' : 'submitting form';

    try {
      const response = await sendTaskCommand({
        command,
        taskId,
        workflowId,
        data,
      }, api);

      if (response.success) {
        navigate(isPreConsignment ? '/pre-consignments' : `/consignments/${workflowId}`);
      } else {
        setSubmitError(response.error?.message || `Failed to ${actionText}.`);
      }
    } catch (err) {
      console.error(`Error ${consoleActionText}:`, err);
      setSubmitError(`Failed to ${actionText}. Please try again.`);
    } finally {
      setIsSubmitting(false);
    }
  };

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    handleFormAction('SUBMISSION');
  };

  const handleSaveAsDraft = () => {
    handleFormAction('SAVE_AS_DRAFT');
  };

  const handleAutoFill = useCallback(() => {
    const filledData = autoFillForm(props.formInfo.schema, data);
    setData(filledData);
  }, [props.formInfo.schema, data]);

  const showAutoFillButton = getBooleanEnv('VITE_SHOW_AUTOFILL_BUTTON', false)

  const isSubmissionFailed = props.pluginState === 'SUBMISSION_FAILED';

  return (
    <>
      {isSubmissionFailed && (
        <div className="bg-amber-50 border border-amber-300 text-amber-800 rounded-lg p-4 mb-4 flex items-start gap-3">
          <svg xmlns="http://www.w3.org/2000/svg" className="w-5 h-5 mt-0.5 shrink-0 text-amber-500" viewBox="0 0 20 20" fill="currentColor">
            <path fillRule="evenodd" d="M8.257 3.099c.765-1.36 2.722-1.36 3.486 0l5.58 9.92c.75 1.334-.213 2.98-1.742 2.98H4.42c-1.53 0-2.493-1.646-1.743-2.98l5.58-9.92zM11 13a1 1 0 11-2 0 1 1 0 012 0zm-1-8a1 1 0 00-1 1v3a1 1 0 002 0V6a1 1 0 00-1-1z" clipRule="evenodd"/>
          </svg>
          <div>
            <p className="font-semibold">Submission failed</p>
            <p className="text-sm mt-0.5">Your previous submission could not be completed. Please review the form and try again.</p>
          </div>
        </div>
      )}

      <div className="bg-white rounded-lg shadow-md p-6 mb-6">
        <h1 className="text-2xl font-bold text-gray-800">{props.formInfo.title}</h1>
      </div>

      <div className="bg-white rounded-lg shadow-md p-6">
        <form onSubmit={handleSubmit} noValidate>
          <JsonForms
            schema={props.formInfo.schema}
            uischema={props.formInfo.uiSchema}
            data={data}
            renderers={radixRenderers}
            readonly={isReadOnly}
            onChange={({ data, errors }) => {
              setData(data);
              setErrors(errors || []);
            }}
          />
          {!isReadOnly && (
            <div className={`mt-4 flex gap-3 ${showAutoFillButton ? 'justify-between' : ''}`}>
              {showAutoFillButton && (
                <Button
                  type="button"
                  variant="soft"
                  color="purple"
                  size={"3"}
                  className={"flex-1!"}
                  onClick={handleAutoFill}
                  disabled={isSubmitting}
                >
                  Demo - Auto Fill
                </Button>
              )}
              <Button
                type="button"
                variant="outline"
                disabled={isSubmitting}
                className={'flex-1!'}
                size={"3"}
                onClick={handleSaveAsDraft}
              >
                Save as Draft
              </Button>
              <Button
                type="submit"
                disabled={isSubmitting}
                className={'flex-1!'}
                size={"3"}
              >
                {isSubmitting ? 'Submitting...' : 'Submit Form'}
              </Button>
            </div>
          )}
        </form>
      </div>

      {submitError && (
        <div className="bg-red-100 text-red-700 rounded-lg p-4 mt-4">
          <p>{submitError}</p>
        </div>
      )}
    </>
  )
}

function SubmissionResponseForm(props: { formInfo: TaskFormData }) {

  return (
    <div className="mt-6 border-l-4 border-emerald-500 rounded-r-lg overflow-hidden shadow-sm">
      <div className="bg-emerald-50 px-6 py-4 flex items-center gap-3">
        <span className="inline-flex items-center justify-center w-7 h-7 rounded-full bg-emerald-100 text-emerald-700 shrink-0">
          <svg xmlns="http://www.w3.org/2000/svg" className="w-4 h-4" viewBox="0 0 20 20" fill="currentColor">
            <path fillRule="evenodd" d="M16.707 5.293a1 1 0 010 1.414l-8 8a1 1 0 01-1.414 0l-4-4a1 1 0 011.414-1.414L8 12.586l7.293-7.293a1 1 0 011.414 0z" clipRule="evenodd" />
          </svg>
        </span>
        <div>
          <p className="text-xs font-semibold uppercase tracking-widest text-emerald-600 mb-0.5">Submission Response</p>
          <h2 className="text-lg font-bold text-emerald-900 leading-tight">{props.formInfo.title}</h2>
        </div>
      </div>

      <div className="bg-white border-t border-emerald-100 p-6">
        <JsonForms
          schema={props.formInfo.schema}
          uischema={props.formInfo.uiSchema}
          data={props.formInfo.formData}
          renderers={radixRenderers}
          readonly={true}
        />
      </div>
    </div>
  )
}

function OgaReviewForm(props: { formInfo: TaskFormData }) {
  const [data] = useState(props.formInfo.formData)

  return (
    <div className="mt-6 rounded-lg overflow-hidden shadow-sm border border-indigo-200">
      <div className="bg-indigo-700 px-6 py-4 flex items-center gap-3">
        <span className="inline-flex items-center justify-center w-7 h-7 rounded-full bg-indigo-600 text-indigo-100 shrink-0">
          <svg xmlns="http://www.w3.org/2000/svg" className="w-4 h-4" viewBox="0 0 20 20" fill="currentColor">
            <path d="M9 2a1 1 0 000 2h2a1 1 0 100-2H9z" />
            <path fillRule="evenodd" d="M4 5a2 2 0 012-2 3 3 0 003 3h2a3 3 0 003-3 2 2 0 012 2v11a2 2 0 01-2 2H6a2 2 0 01-2-2V5zm3 4a1 1 0 000 2h.01a1 1 0 100-2H7zm3 0a1 1 0 000 2h3a1 1 0 100-2h-3zm-3 4a1 1 0 100 2h.01a1 1 0 100-2H7zm3 0a1 1 0 100 2h3a1 1 0 100-2h-3z" clipRule="evenodd" />
          </svg>
        </span>
        <div>
          <p className="text-xs font-semibold uppercase tracking-widest text-indigo-300 mb-0.5">OGA Review</p>
          <h2 className="text-lg font-bold text-white leading-tight">{props.formInfo.title}</h2>
        </div>
      </div>

      <div className="bg-blue-50 border border-blue-200 rounded-lg shadow-md p-6">
        <JsonForms
          schema={props.formInfo.schema}
          uischema={props.formInfo.uiSchema}
          data={data}
          renderers={radixRenderers}
          readonly={true}
        />
      </div>
    </div>
  )
}

// Compact banner shown above the form only when a change is actively requested.
// Displays the single latest feedback entry — just enough to signal action needed.
function LatestFeedbackBanner({ entry }: { entry: OGAFeedbackEntry }) {
  return (
    <div className="mb-6 bg-amber-50 border border-amber-300 rounded-lg p-4 flex items-start gap-3">
      <svg xmlns="http://www.w3.org/2000/svg" className="w-5 h-5 mt-0.5 shrink-0 text-amber-500" viewBox="0 0 20 20" fill="currentColor">
        <path fillRule="evenodd" d="M18 10c0 3.866-3.582 7-8 7a8.841 8.841 0 01-4.083-.98L2 17l1.338-3.123C2.493 12.767 2 11.434 2 10c0-3.866 3.582-7 8-7s8 3.134 8 7zM7 9H5v2h2V9zm8 0h-2v2h2V9zM9 9h2v2H9V9z" clipRule="evenodd" />
      </svg>
      <div className="flex-1 min-w-0">
        <div className="flex items-center justify-between gap-4 mb-1">
          <p className="text-sm font-semibold text-amber-800">Changes Requested</p>
          <span className="text-xs text-amber-600 shrink-0">{new Date(entry.timestamp).toLocaleString()}</span>
        </div>
        <p className="text-sm text-amber-900 whitespace-pre-wrap">{entry.content.feedback as string}</p>
      </div>
    </div>
  )
}

// Full feedback history shown at the bottom in all states where feedback exists.
function OGAFeedbackHistory({ entries }: { entries: OGAFeedbackEntry[] }) {
  return (
    <div className="mt-6 rounded-lg overflow-hidden shadow-sm border border-gray-200">
      <div className="bg-gray-50 px-6 py-3 border-b border-gray-200">
        <p className="text-xs font-semibold uppercase tracking-widest text-gray-500">Review History</p>
      </div>
      <div className="divide-y divide-gray-100">
        {entries.map((entry) => (
          <div key={entry.round} className="bg-white px-6 py-4">
            <div className="flex items-center justify-between mb-1">
              <span className="text-xs font-semibold text-gray-400 uppercase tracking-wider">Round {entry.round}</span>
              <span className="text-xs text-gray-400">{new Date(entry.timestamp).toLocaleString()}</span>
            </div>
            <p className="text-sm text-gray-700 whitespace-pre-wrap">{entry.content.feedback as string}</p>
          </div>
        ))}
      </div>
    </div>
  )
}

export default function SimpleForm(props: { configs: SimpleFormConfig, pluginState: string }) {
  const feedback = props.configs.ogaFeedback
  const latestFeedback = feedback && feedback.length > 0 ? feedback[feedback.length - 1] : null
  const isFeedbackRequested = props.pluginState === 'OGA_FEEDBACK_PROVIDED'

  return (
    <div>
      {isFeedbackRequested && latestFeedback && (
        <LatestFeedbackBanner entry={latestFeedback} />
      )}

      <TraderForm formInfo={props.configs.traderFormInfo} pluginState={props.pluginState} />

      {props.configs.submissionResponseForm && (
        <SubmissionResponseForm formInfo={props.configs.submissionResponseForm} />
      )}

      {props.configs.ogaReviewForm && (
        <OgaReviewForm formInfo={props.configs.ogaReviewForm} />
      )}

      {feedback && feedback.length > 0 && (
        <OGAFeedbackHistory entries={feedback} />
      )}
    </div>
  )
}
