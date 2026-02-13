import { useState } from 'react';
import type {
  JsonFormProps,
  JsonSchema,
  JsonSchemaProperty,
  UISchemaElement,
  Layout,
  ControlElement,
  LabelElement,
  Categorization,
  ResolvedControl,
} from './types';
import {
  TextField,
  NumberField,
  TextareaField,
  SelectField,
  CheckboxField,
  FileField,
} from './fields';

// Helper to get nested value from object using dot notation
function getNestedValue(obj: Record<string, unknown>, path: string): unknown {
  const keys = path.split('.');
  let value: unknown = obj;
  for (const key of keys) {
    if (value && typeof value === 'object' && key in value) {
      value = (value as Record<string, unknown>)[key];
    } else {
      return undefined;
    }
  }
  return value;
}

// Parse scope to navigate nested properties
// e.g., "#/properties/header/properties/declarationType" -> ["header", "declarationType"]
function parseScopePath(scope: string): string[] {
  const path = scope.replace(/^#\//, '').split('/');
  // Filter out "properties" keywords, keep only actual property names
  return path.filter(segment => segment !== 'properties');
}

// Resolve a control element to get its schema property and metadata
function resolveControl(
  control: ControlElement,
  schema: JsonSchema
): ResolvedControl | null {
  const path = parseScopePath(control.scope);

  if (path.length === 0) return null;

  // Navigate through nested schema properties
  let currentSchema: JsonSchema | JsonSchemaProperty = schema;
  let property: JsonSchemaProperty | undefined;

  for (let i = 0; i < path.length; i++) {
    const segment = path[i];
    property = currentSchema.properties?.[segment];

    if (!property) {
      console.log(`Property not found: ${control.scope}`, { path, segment, currentSchema });
      return null;
    }

    // Move deeper if not at the end
    if (i < path.length - 1) {
      currentSchema = property;
    }
  }

  if (!property) return null;

  // Get the leaf property name
  const propertyName = path[path.length - 1];

  // Check if required in the immediate parent schema
  const parentPath = path.slice(0, -1);
  let parentSchema: JsonSchema | JsonSchemaProperty = schema;
  for (const segment of parentPath) {
    parentSchema = parentSchema.properties?.[segment] as JsonSchema;
  }

  const required = parentSchema.required?.includes(propertyName) ?? false;

  // Determine label
  let label: string;
  if (control.label === false) {
    label = '';
  } else if (typeof control.label === 'string') {
    label = control.label;
  } else {
    label = property.title ?? propertyName;
  }

  // Create a unique name for form state using full path
  const fullName = path.join('.');

  return {
    name: fullName,
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
    if (options.format === 'file') return 'file';
    return options.format;
  }

  // Check for file type
  if (property.format === 'file') {
    return 'file';
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
  readOnly?: boolean;
}

function renderElement({
  element,
  schema,
  values,
  errors,
  touched,
  setValue,
  setTouched,
  readOnly,
}: RenderElementProps): React.ReactNode {
  switch (element.type) {
    case 'VerticalLayout':
    case 'HorizontalLayout':
      return renderLayout(element, {
        schema,
        values,
        errors,
        touched,
        setValue,
        setTouched,
        readOnly,
      });

    case 'Group':
      return renderGroup(element, {
        schema,
        values,
        errors,
        touched,
        setValue,
        setTouched,
        readOnly,
      });

    case 'Categorization':
      return renderCategorization(element, {
        schema,
        values,
        errors,
        touched,
        setValue,
        setTouched,
        readOnly,
      });

    case 'Control':
      return renderControl(element, {
        schema,
        values,
        errors,
        touched,
        setValue,
        setTouched,
        readOnly,
      });

    case 'Label':
      return renderLabel(element);

    default:
      console.log("Unknown element type:", element);
      return null;
  }
}

function renderLayout(
  layout: Layout,
  props: Omit<RenderElementProps, 'element'>
): React.ReactNode {
  const isHorizontal = layout.type === 'HorizontalLayout';

  return (
    <div className={isHorizontal ? 'flex gap-4' : 'space-y-4'}>
      {layout.elements.map((element, index) => (
        <div key={index} className={isHorizontal ? 'flex-1' : ''}>
          {renderElement({ element, ...props })}
        </div>
      ))}
    </div>
  );
}

function renderGroup(
  group: Layout,
  props: Omit<RenderElementProps, 'element'>
): React.ReactNode {
  const content = (
    <>
      {group.elements.map((element, index) => (
        <div className="Hello Group" key={index}>
          {renderElement({ element, ...props })}
        </div>
      ))}
    </>
  );

  if (group.label) {
    return (
      <fieldset className="border border-gray-200 rounded-md p-4 mb-4">
        <legend className="text-sm font-medium text-gray-700 px-2">
          {group.label}
        </legend>
        <div className="space-y-4">
          {content}
        </div>
      </fieldset>
    );
  }

  return <div className="space-y-4">{content}</div>;
}

function renderCategorization(
  categorization: Categorization,
  props: Omit<RenderElementProps, 'element'>
): React.ReactNode {
  const [activeTab, setActiveTab] = useState(0);

  const categories = categorization.elements;

  return (
    <div className="mb-4">
      {/* Tab Headers */}
      <div className="border-b border-gray-200">
        <nav className="-mb-px flex gap-6" aria-label="Tabs">
          {categories.map((category, index) => (
            <button
              key={index}
              type="button"
              onClick={() => setActiveTab(index)}
              className={`
                whitespace-nowrap py-3 px-1 border-b-2 font-medium text-sm transition-colors
                ${
                  activeTab === index
                    ? 'border-blue-500 text-blue-600'
                    : 'border-transparent text-gray-500 hover:text-gray-700 hover:border-gray-300'
                }
              `}
            >
              {category.label}
            </button>
          ))}
        </nav>
      </div>

      {/* Tab Content */}
      <div className="mt-4">
        {categories.map((category, index) => (
          <div
            key={index}
            className={activeTab === index ? 'block' : 'hidden'}
          >
            {category.elements.map((element, elemIndex) => (
              <div key={elemIndex}>
                {renderElement({ element, ...props })}
              </div>
            ))}
          </div>
        ))}
      </div>
    </div>
  );
}

function renderControl(
  control: ControlElement,
  props: Omit<RenderElementProps, 'element'>
): React.ReactNode {
  const { schema, values, errors, touched, setValue, setTouched, readOnly } = props;
  const resolved = resolveControl(control, schema);

  if (!resolved) return null;

  const fieldType = getFieldType(resolved);
  const fieldProps = {
    control: resolved,
    value: getNestedValue(values, resolved.name),
    error: errors[resolved.name],
    touched: touched[resolved.name] ?? false,
    onChange: (value: unknown) => setValue(resolved.name, value),
    onBlur: () => setTouched(resolved.name),
    readOnly: readOnly || resolved.options?.readonly,
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
    case 'file':
      return <FileField key={resolved.name} {...fieldProps} />;
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
  uiSchema,
  values,
  errors,
  touched,
  setValue,
  setTouched,
  className = '',
  readOnly = false,
}: JsonFormProps) {
  return (
    <div className={className}>
      {schema.title && (
        <h2 className="text-xl font-semibold mb-4 text-gray-900">
          {schema.title}
        </h2>
      )}

      {uiSchema && renderElement({
        element: uiSchema,
        schema,
        values,
        errors,
        touched,
        setValue,
        setTouched,
        readOnly,
      })}
    </div>
  );
}