import { useState, useCallback, useMemo, useEffect } from 'react';
import type { JsonSchema, FormValues, FormErrors, FormTouched, FormState } from './types';
import { schemaToZod, validateProperty } from './schemaToZod';
import { SAMPLE_DATA_MAP } from './sampleData';

// Helper to set nested value in object using dot notation
function setNestedValue(obj: Record<string, unknown>, path: string, value: unknown): Record<string, unknown> {
  const keys = path.split('.');
  const newObj = { ...obj };
  let current: Record<string, unknown> = newObj;

  for (let i = 0; i < keys.length - 1; i++) {
    const key = keys[i];
    if (!current[key] || typeof current[key] !== 'object') {
      current[key] = {};
    }
    current = current[key] as Record<string, unknown>;
  }

  current[keys[keys.length - 1]] = value;
  return newObj;
}

interface UseJsonFormOptions {
  schema: JsonSchema;
  data?: FormValues;
  onSubmit: (values: FormValues) => void | Promise<void>;
}

interface UseJsonFormReturn extends FormState {
  setValue: (name: string, value: unknown) => void;
  setTouched: (name: string) => void;
  validateField: (name: string) => string | undefined;
  validateForm: () => boolean;
  handleSubmit: (e: React.FormEvent) => Promise<void>;
  reset: () => void;
  autoFillForm: () => void;
  setValues: (values: FormValues) => void;
}

function getInitialValues(schema: JsonSchema, data?: FormValues): FormValues {
  const values: FormValues = {};
  const requiredFields = new Set(schema.required ?? []);

  if (schema.properties) {
    for (const [name, property] of Object.entries(schema.properties)) {
      if (data?.[name] !== undefined) {
        values[name] = data[name];
      } else if (property.default !== undefined) {
        values[name] = property.default;
      } else if (property.type === 'object' && property.properties) {
        // Recursively initialize nested objects
        values[name] = getInitialValues(
          property as JsonSchema,
          (data?.[name] as FormValues | undefined) || undefined
        );
      } else {
        // Set appropriate default based on type
        switch (property.type) {
          case 'boolean':
            values[name] = requiredFields.has(name) ? false : false;
            break;
          case 'number':
          case 'integer':
            values[name] = undefined;
            break;
          default:
            values[name] = '';
        }
      }
    }
  }

  return values;
}

export function useJsonForm({
  schema,
  data,
  onSubmit,
}: UseJsonFormOptions): UseJsonFormReturn {
  const defaultValues = useMemo(
    () => getInitialValues(schema, data),
    [schema, data]
  );

  const [values, setValues] = useState<FormValues>(defaultValues);

  useEffect(() => {
    setValues(defaultValues);
  }, [defaultValues]);

  const [errors, setErrors] = useState<FormErrors>({});
  const [touched, setTouchedState] = useState<FormTouched>({});
  const [isSubmitting, setIsSubmitting] = useState(false);

  const zodSchema = useMemo(() => schemaToZod(schema), [schema]);
  const requiredFields = useMemo(() => new Set(schema.required ?? []), [schema]);

  const isValid = useMemo(() => {
    const result = zodSchema.safeParse(values);
    return result.success;
  }, [zodSchema, values]);

  const setValue = useCallback((name: string, value: unknown) => {
    setValues((prev) => setNestedValue(prev, name, value));

    // For validation, we need to navigate to the property in the schema
    const path = name.split('.');
    let property = schema.properties?.[path[0]];

    // Navigate through nested properties
    for (let i = 1; i < path.length && property; i++) {
      property = property.properties?.[path[i]];
    }

    if (property) {
      const leafName = path[path.length - 1];
      // Find parent to check if required
      let parentSchema = schema;
      for (let i = 0; i < path.length - 1; i++) {
        const prop = parentSchema.properties?.[path[i]];
        if (prop) {
          parentSchema = prop as JsonSchema;
        }
      }
      const isRequired = parentSchema.required?.includes(leafName) ?? false;
      const error = validateProperty(property, value, isRequired);
      setErrors((prev) => ({ ...prev, [name]: error }));
    }
  }, [schema]);

  const setTouched = useCallback((name: string) => {
    setTouchedState((prev) => ({ ...prev, [name]: true }));
  }, []);

  const validateField = useCallback(
    (name: string): string | undefined => {
      const property = schema.properties?.[name];
      if (!property) return undefined;

      const error = validateProperty(property, values[name], requiredFields.has(name));
      setErrors((prev) => ({ ...prev, [name]: error }));
      return error;
    },
    [schema.properties, values, requiredFields]
  );

  const validateForm = useCallback((): boolean => {
    const newErrors: FormErrors = {};
    let isFormValid = true;

    if (schema.properties) {
      for (const [name, property] of Object.entries(schema.properties)) {
        const error = validateProperty(property, values[name], requiredFields.has(name));
        if (error) {
          newErrors[name] = error;
          isFormValid = false;
        }
      }
    }

    setErrors(newErrors);

    // Mark all fields as touched
    const allTouched: FormTouched = {};
    if (schema.properties) {
      for (const name of Object.keys(schema.properties)) {
        allTouched[name] = true;
      }
    }
    setTouchedState(allTouched);

    return isFormValid;
  }, [schema.properties, values, requiredFields]);

  const handleSubmit = useCallback(
    async (e: React.FormEvent) => {
      e.preventDefault();

      if (!validateForm()) {
        return;
      }

      setIsSubmitting(true);

      try {
        // Process values - convert string numbers to actual numbers
        const processedValues = { ...values };
        if (schema.properties) {
          for (const [name, property] of Object.entries(schema.properties)) {
            if (
              (property.type === 'number' || property.type === 'integer') &&
              typeof processedValues[name] === 'string'
            ) {
              const numValue = parseFloat(processedValues[name]);
              if (!isNaN(numValue)) {
                processedValues[name] = numValue;
              }
            }
          }
        }

        await onSubmit(processedValues);
      } finally {
        setIsSubmitting(false);
      }
    },
    [validateForm, values, schema.properties, onSubmit]
  );

  const reset = useCallback(() => {
    setValues(defaultValues);
    setErrors({});
    setTouchedState({});
  }, [defaultValues]);

  // Check if a field should be skipped (readonly or from global context)
  const shouldSkipField = useCallback((property: any): boolean => {
    // Skip if marked as readOnly
    if (property.readOnly === true) {
      return true;
    }
    // Skip if it has x-globalContext.readFrom (value comes from backend)
    if (property['x-globalContext']?.readFrom !== "" && property['x-globalContext']?.readFrom !== undefined) {
      return true;
    }
    return false;
  }, []);

  // Generate sample data for a field based on its schema
  const generateSampleValue = useCallback((property: any, fieldName: string): unknown => {
    // Check if this field should be skipped
    if (shouldSkipField(property)) {
      return undefined;
    }

    // First, check the sample data lookup map
    if (fieldName in SAMPLE_DATA_MAP) {
      return SAMPLE_DATA_MAP[fieldName];
    }

    // Check if there's an example in the property
    if (property.example !== undefined) {
      return property.example;
    }

    // Check if there's a description with an example pattern
    if (property.description && typeof property.description === 'string') {
      // Extract example from description if it follows "Example: ..." pattern
      const exampleMatch = property.description.match(/Example:\s*(.+)/i);
      if (exampleMatch) {
        return exampleMatch[1].trim();
      }
    }

    // Handle enum or oneOf (select fields)
    if (property.enum && property.enum.length > 0) {
      return property.enum[0];
    }
    if (property.oneOf && property.oneOf.length > 0) {
      return property.oneOf[0].const;
    }

    // Handle by type
    switch (property.type) {
      case 'boolean':
        return true;
      case 'number':
      case 'integer':
        if (property.minimum !== undefined) {
          return property.minimum;
        }
        if (property.maximum !== undefined) {
          return Math.floor(property.maximum / 2);
        }
        return 100;
      case 'string':
        if (property.format === 'email') {
          return 'test@example.com';
        }
        if (property.format === 'date') {
          return new Date().toISOString().split('T')[0];
        }
        if (property.format === 'date-time') {
          return new Date().toISOString();
        }
        if (property.format === 'file') {
          return new File(['(sample file content)'], 'sample-attachment.pdf', { type: 'application/pdf' });
        }
        if (property.enum) {
          return property.enum[0];
        }
        // Fall back to a generic sample value
        const label = property.title || fieldName;
        return `Sample ${label}`;
      case 'object':
        // Recursively generate nested objects
        if (property.properties) {
          const nestedObj: Record<string, unknown> = {};
          for (const [nestedName, nestedProperty] of Object.entries(property.properties)) {
            const value = generateSampleValue(nestedProperty, nestedName);
            if (value !== undefined) {
              nestedObj[nestedName] = value;
            }
          }
          return nestedObj;
        }
        return {};
      case 'array':
        return [];
      default:
        return `Sample ${fieldName}`;
    }
  }, [shouldSkipField]);

  // Auto-fill empty fields with sample data
  const autoFillForm = useCallback(() => {
    const newValues = { ...values };

    // Helper to check if a value is empty
    const isEmpty = (val: unknown): boolean => {
      return val === undefined || val === null || val === '';
    };

    // Helper to recursively auto-fill nested objects
    const fillNestedValues = (
      currentSchema: JsonSchema,
      currentValues: Record<string, unknown>,
      path: string[] = []
    ): Record<string, unknown> => {
      const result = { ...currentValues };

      if (currentSchema.properties) {
        for (const [name, property] of Object.entries(currentSchema.properties)) {
          // Skip fields that should not be auto-filled
          if (shouldSkipField(property)) {
            continue;
          }

          if (property.type === 'object' && property.properties) {
            // Recursively fill nested objects
            const nestedValues = (result[name] as Record<string, unknown>) || {};
            result[name] = fillNestedValues(
              property as JsonSchema,
              nestedValues,
              [...path, name]
            );
          } else if (isEmpty(result[name])) {
            // Only fill if the field is empty
            const value = generateSampleValue(property, name);
            if (value !== undefined) {
              result[name] = value;
            }
          }
        }
      }

      return result;
    };

    const filledValues = fillNestedValues(schema, newValues);
    setValues(filledValues);
  }, [values, schema, generateSampleValue, shouldSkipField]);

  return {
    values,
    errors,
    touched,
    isSubmitting,
    isValid,
    setValue,
    setTouched,
    validateField,
    validateForm,
    handleSubmit,
    reset,
    autoFillForm,
    setValues,
  };
}