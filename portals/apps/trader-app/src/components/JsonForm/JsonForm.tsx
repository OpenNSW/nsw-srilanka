import type {
  JsonFormProps,
  JsonSchema,
  UISchemaElement,
  Layout,
  ControlElement,
  LabelElement,
  ResolvedControl,
} from './types';
import { useJsonForm } from './useJsonForm';
import {
  TextField,
  NumberField,
  TextareaField,
  SelectField,
  CheckboxField,
} from './fields';

// Parse scope to get property name (e.g., "#/properties/name" -> "name")
function scopeToPropertyName(scope: string): string {
  const match = scope.match(/#\/properties\/(.+)/);
  return match ? match[1] : scope;
}

// Resolve a control element to get its schema property and metadata
function resolveControl(
  control: ControlElement,
  schema: JsonSchema
): ResolvedControl | null {
  const propertyName = scopeToPropertyName(control.scope);
  const property = schema.properties?.[propertyName];

  if (!property) return null;

  const required = schema.required?.includes(propertyName) ?? false;

  // Determine label
  let label: string;
  if (control.label === false) {
    label = '';
  } else if (typeof control.label === 'string') {
    label = control.label;
  } else {
    label = property.title ?? propertyName;
  }

  return {
    name: propertyName,
    label,
    property,
    required,
    options: control.options,
  };
}

// Determine field type based on schema property
function getFieldType(control: ResolvedControl): string {
  const { property, options } = control;

  // Check for explicit format override in options
  if (options?.format) {
    return options.format;
  }

  // Check for textarea (multi-line string)
  if (options?.multi || (options?.rows && options.rows > 1)) {
    return 'textarea';
  }

  // Check for select (enum or oneOf)
  if (property.enum || property.oneOf) {
    return 'select';
  }

  // Check by type
  switch (property.type) {
    case 'boolean':
      return 'checkbox';
    case 'number':
    case 'integer':
      return 'number';
    case 'string':
      if (property.format === 'email') return 'email';
      return 'text';
    default:
      return 'text';
  }
}

interface RenderElementProps {
  element: UISchemaElement;
  schema: JsonSchema;
  values: Record<string, unknown>;
  errors: Record<string, string | undefined>;
  touched: Record<string, boolean>;
  setValue: (name: string, value: unknown) => void;
  setTouched: (name: string) => void;
}

function renderElement({
  element,
  schema,
  values,
  errors,
  touched,
  setValue,
  setTouched,
}: RenderElementProps): React.ReactNode {
  switch (element.type) {
    case 'VerticalLayout':
    case 'HorizontalLayout':
    case 'Group':
      return renderLayout(element as Layout, {
        schema,
        values,
        errors,
        touched,
        setValue,
        setTouched,
      });

    case 'Control':
      return renderControl(element as ControlElement, {
        schema,
        values,
        errors,
        touched,
        setValue,
        setTouched,
      });

    case 'Label':
      return renderLabel(element as LabelElement);

    default:
      return null;
  }
}

function renderLayout(
  layout: Layout,
  props: Omit<RenderElementProps, 'element'>
): React.ReactNode {
  const isHorizontal = layout.type === 'HorizontalLayout';
  const isGroup = layout.type === 'Group';

  const content = (
    <div
      className={`${isHorizontal ? 'flex gap-4' : ''}`}
    >
      {layout.elements.map((element, index) => (
        <div key={index} className={isHorizontal ? 'flex-1' : ''}>
          {renderElement({ element, ...props })}
        </div>
      ))}
    </div>
  );

  if (isGroup && layout.label) {
    return (
      <fieldset className="border border-gray-200 rounded-md p-4 mb-4">
        <legend className="text-sm font-medium text-gray-700 px-2">
          {layout.label}
        </legend>
        {content}
      </fieldset>
    );
  }

  return content;
}

function renderControl(
  control: ControlElement,
  props: Omit<RenderElementProps, 'element'>
): React.ReactNode {
  const { schema, values, errors, touched, setValue, setTouched } = props;
  const resolved = resolveControl(control, schema);

  if (!resolved) return null;

  const fieldType = getFieldType(resolved);
  const fieldProps = {
    control: resolved,
    value: values[resolved.name],
    error: errors[resolved.name],
    touched: touched[resolved.name] ?? false,
    onChange: (value: unknown) => setValue(resolved.name, value),
    onBlur: () => setTouched(resolved.name),
  };

  switch (fieldType) {
    case 'text':
    case 'email':
      return <TextField key={resolved.name} {...fieldProps} />;
    case 'number':
      return <NumberField key={resolved.name} {...fieldProps} />;
    case 'textarea':
      return <TextareaField key={resolved.name} {...fieldProps} />;
    case 'select':
      return <SelectField key={resolved.name} {...fieldProps} />;
    case 'checkbox':
      return <CheckboxField key={resolved.name} {...fieldProps} />;
    default:
      return <TextField key={resolved.name} {...fieldProps} />;
  }
}

function renderLabel(label: LabelElement): React.ReactNode {
  return (
    <p className="text-sm font-medium text-gray-700 mb-2">{label.text}</p>
  );
}

export function JsonForm({
  schema,
  uischema,
  data,
  onSubmit,
  onSaveDraft,
  submitLabel = 'Submit',
  draftLabel = 'Save Draft',
  showDraftButton = false,
  className = '',
}: JsonFormProps) {
  const form = useJsonForm({ schema, data, onSubmit });

  const handleSaveDraft = () => {
    if (onSaveDraft) {
      onSaveDraft(form.values);
    }
  };

  return (
    <form
      onSubmit={form.handleSubmit}
      className={className}
      noValidate
    >
      {schema.title && (
        <h2 className="text-xl font-semibold mb-4 text-gray-900">
          {schema.title}
        </h2>
      )}

      {renderElement({
        element: uischema,
        schema,
        values: form.values,
        errors: form.errors,
        touched: form.touched,
        setValue: form.setValue,
        setTouched: form.setTouched,
      })}

      <div className={`mt-4 flex gap-3 ${showDraftButton ? 'justify-between' : ''}`}>
        {showDraftButton && onSaveDraft && (
          <button
            type="button"
            onClick={handleSaveDraft}
            disabled={form.isSubmitting}
            className={`
              px-4 py-2 font-medium rounded-md
              border border-gray-300 text-gray-700 bg-white
              hover:bg-gray-50
              focus:outline-none focus:ring-2 focus:ring-blue-500 focus:ring-offset-2
              disabled:bg-gray-100 disabled:cursor-not-allowed
              transition-colors
            `}
          >
            {draftLabel}
          </button>
        )}
        <button
          type="submit"
          disabled={form.isSubmitting}
          className={`
            ${showDraftButton ? 'flex-1' : 'w-full'} px-4 py-2 text-white font-medium rounded-md
            bg-blue-600 hover:bg-blue-700
            focus:outline-none focus:ring-2 focus:ring-blue-500 focus:ring-offset-2
            disabled:bg-blue-400 disabled:cursor-not-allowed
            transition-colors
          `}
        >
          {form.isSubmitting ? 'Submitting...' : submitLabel}
        </button>
      </div>
    </form>
  );
}
