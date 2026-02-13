import type { FieldProps } from '../types';
import { FieldWrapper } from './FieldWrapper';

export function TextField({ control, value, error, touched, onChange, onBlur, readOnly }: FieldProps) {
  const inputType = control.property.format === 'email' ? 'email' : 'text';
  const isReadonly = readOnly ?? control.options?.readonly;

  return (
    <FieldWrapper control={control} error={error} touched={touched}>
      <input
        type={inputType}
        name={control.name}
        value={(value as string) ?? ''}
        onChange={(e) => onChange(e.target.value)}
        onBlur={onBlur}
        placeholder={control.options?.placeholder}
        disabled={isReadonly}
        className={`
          w-full px-3 py-2 border rounded-md shadow-sm
          focus:outline-none focus:ring-2 focus:ring-blue-500 focus:border-blue-500
          disabled:bg-gray-100 disabled:cursor-not-allowed
          ${touched && error ? 'border-red-500' : 'border-gray-300'}
        `}
      />
    </FieldWrapper>
  );
}
