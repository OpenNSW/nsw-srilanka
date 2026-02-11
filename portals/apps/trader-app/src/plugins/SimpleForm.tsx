import {JsonForm, useJsonForm, type JsonSchema, type UISchemaElement} from "../components/JsonForm";
import {sendTaskCommand} from "../services/task.ts";
import {useNavigate, useParams} from "react-router-dom";
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
}

export default function SimpleForm(props: { configs: SimpleFormConfig, pluginState: string }) {
  const {consignmentId, taskId} = useParams<{
    consignmentId: string
    taskId: string
  }>()
  const navigate = useNavigate()
  const [error, setError] = useState<string | null>(null)

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
        workflowId:   consignmentId,
        data: data as Record<string, unknown>,
      })

      if (response.success) {
        // Navigate back to consignment details
        navigate(`/consignments/${consignmentId}`)
      } else {
        setError(response.message || 'Failed to submit form.')
      }
    } catch (err) {
      console.error('Error submitting form:', err)
      setError('Failed to submit form. Please try again.')
    }
  }

  const form = useJsonForm({
    schema: props.configs.traderFormInfo.schema,
    data: props.configs.traderFormInfo.formData,
    onSubmit: handleSubmit,
  })

  const showAutoFillButton = import.meta.env.VITE_SHOW_AUTOFILL_BUTTON === 'true'

  return (
    <div>
      <div className="bg-white rounded-lg shadow-md p-6 mb-6">
        <h1 className="text-2xl font-bold text-gray-800">{props.configs.traderFormInfo.title}</h1>
      </div>

      <div className="bg-white rounded-lg shadow-md p-6">
        <form onSubmit={form.handleSubmit} noValidate>
          <JsonForm
            schema={props.configs.traderFormInfo.schema}
            uiSchema={props.configs.traderFormInfo.uiSchema}
            values={form.values}
            errors={form.errors}
            touched={form.touched}
            setValue={form.setValue}
            setTouched={form.setTouched}
          />
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
                Auto-Fill Form
              </Button>
            )}
            <Button
              type="submit"
              disabled={form.isSubmitting || (props.pluginState !== "INITIALIZED" && props.pluginState !== "DRAFT")}
              className={'flex-1!'}
              size={"3"}
            >
              {form.isSubmitting ? 'Submitting...' : 'Submit Form'}
            </Button>
          </div>
        </form>
      </div>
      {error && (
        <div className="bg-red-100 text-red-700 rounded-lg p-4 mt-4">
          <p>{error}</p>
        </div>
      )}
    </div>
  )
}