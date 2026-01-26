import { useState, useCallback, useMemo } from 'react';
import type { JsonSchema, FormValues, FormErrors, FormTouched, FormState } from './types';
import { schemaToZod, validateProperty } from './schemaToZod';

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
    setValues((prev) => ({ ...prev, [name]: value }));

    const property = schema.properties?.[name];
    if (property) {
      const error = validateProperty(property, value, requiredFields.has(name));
      setErrors((prev) => ({ ...prev, [name]: error }));
    }
  }, [schema.properties, requiredFields]);

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
              const numValue = parseFloat(processedValues[name] as string);
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
  };
}
