import type { FieldProps } from '../types';
import { FieldWrapper } from './FieldWrapper';

export function TextareaField({ control, value, error, touched, onChange, onBlur }: FieldProps) {
  const isReadonly = control.options?.readonly;
  const rows = control.options?.rows ?? 3;

  return (
    <FieldWrapper control={control} error={error} touched={touched}>
      <textarea
        name={control.name}
        value={(value as string) ?? ''}
        onChange={(e) => onChange(e.target.value)}
        onBlur={onBlur}
        placeholder={control.options?.placeholder}
        disabled={isReadonly}
        rows={rows}
        className={`
          w-full px-3 py-2 border rounded-md shadow-sm resize-y
          focus:outline-none focus:ring-2 focus:ring-blue-500 focus:border-blue-500
          disabled:bg-gray-100 disabled:cursor-not-allowed
          ${touched && error ? 'border-red-500' : 'border-gray-300'}
        `}
      />
    </FieldWrapper>
  );
}
