import {JsonForm, type JsonSchema, type UISchemaElement, useJsonForm} from "../components/JsonForm";
import {sendTaskCommand} from "../services/task.ts";
import {uploadFile} from "../services/upload";
import {useLocation, useNavigate, useParams} from "react-router-dom";
import {useState} from "react";
import {Button} from "@radix-ui/themes";


export interface TaskFormData {
  title: string
  schema: JsonSchema
  uiSchema: UISchemaElement
  formData: Record<string, unknown>
}

export type SimpleFormConfig = {
  traderFormInfo: TaskFormData
  ogaReviewForm?: TaskFormData
}

function TraderForm(props: { formInfo: TaskFormData, pluginState: string }) {
  const { consignmentId, preConsignmentId, taskId } = useParams<{
    consignmentId?: string
    preConsignmentId?: string
    taskId?: string
  }>()
  const location = useLocation()
  const navigate = useNavigate()
  const [error, setError] = useState<string | null>(null)

  const READ_ONLY_STATES = ['OGA_REVIEWED', 'SUBMITTED', 'OGA_ACKNOWLEDGED'];
  const isReadOnly = READ_ONLY_STATES.includes(props.pluginState);

  const isPreConsignment = location.pathname.includes('/pre-consignments/')
  const workflowId = preConsignmentId || consignmentId

  const replaceFilesWithKeys = async (value: unknown): Promise<unknown> => {
    if (value instanceof File) {
      const metadata = await uploadFile(value)
      return metadata.key
    }

    if (Array.isArray(value)) {
      return await Promise.all(value.map(replaceFilesWithKeys))
    }

    if (value && typeof value === 'object') {
      const entries = await Promise.all(
        Object.entries(value as Record<string, unknown>).map(async ([key, nested]) => [
          key,
          await replaceFilesWithKeys(nested),
        ] as const)
      )
      return Object.fromEntries(entries)
    }

    return value
  }

  const handleSubmit = async (data: unknown) => {
    if (!workflowId || !taskId) {
      setError('Workflow ID or Task ID is missing.')
      return
    }

    try {
      setError(null)

      // Send form submission - data now contains file keys (strings) instead of File objects
      const preparedData = await replaceFilesWithKeys(data) as Record<string, unknown>

      const response = await sendTaskCommand({
        command: 'SUBMISSION',
        taskId,
        workflowId,
        data: preparedData,
      })

      if (response.success) {
        // Navigate back to appropriate workflow list
        navigate(isPreConsignment ? '/pre-consignments' : `/consignments/${workflowId}`)
      } else {
        setError(response.error?.message || 'Failed to submit form.')
      }
    } catch (err) {
      console.error('Error submitting form:', err)
      setError('Failed to submit form. Please try again.')
    }
  }

  const form = useJsonForm({
    schema: props.formInfo.schema,
    data: props.formInfo.formData,
    onSubmit: handleSubmit,
  })

  const showAutoFillButton = import.meta.env.VITE_SHOW_AUTOFILL_BUTTON === 'true'

  return (
    <>
      <div className="bg-white rounded-lg shadow-md p-6 mb-6">
        <h1 className="text-2xl font-bold text-gray-800">{props.formInfo.title}</h1>
      </div>

      <div className="bg-white rounded-lg shadow-md p-6">
        <form onSubmit={form.handleSubmit} noValidate>
          <JsonForm
            schema={props.formInfo.schema}
            uiSchema={props.formInfo.uiSchema}
            values={form.values}
            errors={form.errors}
            touched={form.touched}
            setValue={form.setValue}
            setTouched={form.setTouched}
            readOnly={isReadOnly}
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
                  onClick={form.autoFillForm}
                  disabled={form.isSubmitting}
                >
                  Demo - Auto Fill
                </Button>
              )}
              <Button
                type="submit"
                disabled={form.isSubmitting}
                className={'flex-1!'}
                size={"3"}
              >
                {form.isSubmitting ? 'Submitting...' : 'Submit Form'}
              </Button>
            </div>
          )}
        </form>
      </div>

      {error && (
        <div className="bg-red-100 text-red-700 rounded-lg p-4 mt-4">
          <p>{error}</p>
        </div>
      )}
    </>
  )
}

function OgaReviewForm(props: { formInfo: TaskFormData }) {
  const form = useJsonForm({
    schema: props.formInfo.schema,
    data: props.formInfo.formData,
    onSubmit: () => {},
  })

  return (
    <>
      <div className="bg-blue-50 border border-blue-200 rounded-lg shadow-md p-6 mb-6 mt-6">
        <h1 className="text-2xl font-bold text-blue-800">{props.formInfo.title}</h1>
      </div>

      <div className="bg-blue-50 border border-blue-200 rounded-lg shadow-md p-6">
        <JsonForm
          schema={props.formInfo.schema}
          uiSchema={props.formInfo.uiSchema}
          values={form.values}
          errors={form.errors}
          touched={form.touched}
          setValue={form.setValue}
          setTouched={form.setTouched}
          readOnly={true}
        />
      </div>
    </>
  )
}

export default function SimpleForm(props: { configs: SimpleFormConfig, pluginState: string }) {
  return (
    <div>
      <TraderForm formInfo={props.configs.traderFormInfo} pluginState={props.pluginState} />

      {props.configs.ogaReviewForm && (
        <OgaReviewForm formInfo={props.configs.ogaReviewForm} />
      )}
    </div>
  )
}
