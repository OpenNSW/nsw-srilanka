// JSON Schema types (following JSON Schema draft-07)
export interface JsonSchema {
  type?: 'string' | 'number' | 'integer' | 'boolean' | 'object' | 'array';
  properties?: Record<string, JsonSchemaProperty>;
  required?: string[];
  title?: string;
  description?: string;
}

export interface JsonSchemaProperty {
  type?: 'string' | 'number' | 'integer' | 'boolean' | 'array';
  title?: string;
  description?: string;
  default?: unknown;
  // String validations
  minLength?: number;
  maxLength?: number;
  pattern?: string;
  format?: 'email' | 'uri' | 'date' | 'date-time' | 'time';
  // Number validations
  minimum?: number;
  maximum?: number;
  exclusiveMinimum?: number;
  exclusiveMaximum?: number;
  multipleOf?: number;
  // Enum for select fields
  enum?: (string | number)[];
  // Array for oneOf select options
  oneOf?: { const: string | number; title: string }[];
}

// UI Schema types (following JSON Forms standard)
export type UISchemaElement = Layout | ControlElement | LabelElement;

export interface Layout {
  type: 'VerticalLayout' | 'HorizontalLayout' | 'Group';
  label?: string;
  elements: UISchemaElement[];
}

export interface ControlElement {
  type: 'Control';
  scope: string; // JSON Pointer to schema property, e.g., "#/properties/name"
  label?: string | boolean;
  options?: ControlOptions;
}

export interface LabelElement {
  type: 'Label';
  text: string;
}

export interface ControlOptions {
  placeholder?: string;
  multi?: boolean; // for textarea
  rows?: number; // for textarea
  readonly?: boolean;
  format?: string; // override format detection
  showUnfocusedDescription?: boolean;
}

// Form state types
export type FormValues = Record<string, unknown>;
export type FormErrors = Record<string, string | undefined>;
export type FormTouched = Record<string, boolean>;

export interface FormState {
  values: FormValues;
  errors: FormErrors;
  touched: FormTouched;
  isSubmitting: boolean;
  isValid: boolean;
}

// Resolved control with schema info for rendering
export interface ResolvedControl {
  name: string;
  label: string;
  property: JsonSchemaProperty;
  required: boolean;
  options?: ControlOptions;
}

// Field component props
export interface FieldProps {
  control: ResolvedControl;
  value: unknown;
  error?: string;
  touched: boolean;
  onChange: (value: unknown) => void;
  onBlur: () => void;
}

// Main component props
export interface JsonFormProps {
  schema: JsonSchema;
  uischema: UISchemaElement;
  data?: FormValues;
  onSubmit: (values: FormValues) => void | Promise<void>;
  onSaveDraft?: (values: FormValues) => void | Promise<void>;
  submitLabel?: string;
  draftLabel?: string;
  showDraftButton?: boolean;
  className?: string;
}
