import { z } from 'zod';
import type { ZodTypeAny } from 'zod';
import type { JsonSchema, JsonSchemaProperty } from './types';

function propertyToZod(property: JsonSchemaProperty, required: boolean): ZodTypeAny {
  const { type, format } = property;

  switch (type) {
    case 'string': {
      let schema = z.string();

      if (required) {
        schema = schema.min(1, 'This field is required');
      }

      if (property.minLength !== undefined) {
        schema = schema.min(property.minLength, `Minimum ${property.minLength} characters required`);
      }

      if (property.maxLength !== undefined) {
        schema = schema.max(property.maxLength, `Maximum ${property.maxLength} characters allowed`);
      }

      if (format === 'email') {
        schema = schema.email('Please enter a valid email address');
      }

      if (property.pattern) {
        schema = schema.regex(new RegExp(property.pattern), 'Invalid format');
      }

      // Handle enum as string validation
      if (property.enum && property.enum.length > 0) {
        const enumValues = property.enum.filter((v): v is string => typeof v === 'string');
        if (enumValues.length > 0) {
          return required
            ? z.enum(enumValues as [string, ...string[]])
            : z.enum(enumValues as [string, ...string[]]).optional();
        }
      }

      return required ? schema : schema.optional();
    }

    case 'number':
    case 'integer': {
      let schema = z.number({
        required_error: 'This field is required',
        invalid_type_error: 'Please enter a valid number',
      });

      if (type === 'integer') {
        schema = schema.int('Please enter a whole number');
      }

      if (property.minimum !== undefined) {
        schema = schema.min(property.minimum, `Minimum value is ${property.minimum}`);
      }

      if (property.maximum !== undefined) {
        schema = schema.max(property.maximum, `Maximum value is ${property.maximum}`);
      }

      if (property.exclusiveMinimum !== undefined) {
        schema = schema.gt(property.exclusiveMinimum, `Value must be greater than ${property.exclusiveMinimum}`);
      }

      if (property.exclusiveMaximum !== undefined) {
        schema = schema.lt(property.exclusiveMaximum, `Value must be less than ${property.exclusiveMaximum}`);
      }

      if (property.multipleOf !== undefined) {
        schema = schema.multipleOf(property.multipleOf);
      }

      return required ? schema : schema.optional();
    }

    case 'boolean': {
      const schema = z.boolean();
      if (required) {
        return schema.refine((val) => val === true, {
          message: 'This field is required',
        });
      }
      return schema;
    }

    default:
      return required ? z.unknown() : z.unknown().optional();
  }
}

export function schemaToZod(jsonSchema: JsonSchema): z.ZodObject<Record<string, ZodTypeAny>> {
  const shape: Record<string, ZodTypeAny> = {};
  const requiredFields = new Set(jsonSchema.required ?? []);

  if (jsonSchema.properties) {
    for (const [name, property] of Object.entries(jsonSchema.properties)) {
      shape[name] = propertyToZod(property, requiredFields.has(name));
    }
  }

  return z.object(shape);
}

export function validateProperty(
  property: JsonSchemaProperty,
  value: unknown,
  required: boolean
): string | undefined {
  const zodSchema = propertyToZod(property, required);
  const result = zodSchema.safeParse(value);

  if (!result.success) {
    return result.error.errors[0]?.message;
  }

  return undefined;
}
