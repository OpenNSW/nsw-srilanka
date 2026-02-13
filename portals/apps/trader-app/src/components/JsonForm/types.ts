// JSON Schema types (following JSON Schema draft-07)
export interface JsonSchema {
  type?: 'string' | 'number' | 'integer' | 'boolean' | 'object' | 'array';
  properties?: Record<string, JsonSchemaProperty>;
  required?: string[];
  title?: string;
  description?: string;
}

export interface JsonSchemaProperty {
  type?: 'string' | 'number' | 'integer' | 'boolean' | 'array' | 'object';
  title?: string;
  description?: string;
  default?: unknown;
  // String validations
  minLength?: number;
  maxLength?: number;
  pattern?: string;
  format?: 'email' | 'uri' | 'date' | 'date-time' | 'time' | 'file';
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
  // Object properties (for nested schemas)
  properties?: Record<string, JsonSchemaProperty>;
  required?: string[];
}

// UI Schema types (following JSON Forms standard)
export type UISchemaElement = Layout | ControlElement | LabelElement | Categorization;

export interface Layout {
  type: 'VerticalLayout' | 'HorizontalLayout' | 'Group';
  label?: string;
  elements: UISchemaElement[];
}

export interface Categorization {
  type: 'Categorization';
  elements: Category[];
  label?: string;
}

export interface Category {
  type: 'Category';
  label: string;
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
  maxSize?: number; // for file upload (in bytes)
  accept?: string; // for file upload (MIME types or extensions)
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
  readOnly?: boolean;
}

// Main component props
export interface JsonFormProps {
  schema: JsonSchema;
  uiSchema?: UISchemaElement;
  values: FormValues;
  errors: FormErrors;
  touched: FormTouched;
  setValue: (name: string, value: unknown) => void;
  setTouched: (name: string) => void;
  className?: string;
  readOnly?: boolean;
}
