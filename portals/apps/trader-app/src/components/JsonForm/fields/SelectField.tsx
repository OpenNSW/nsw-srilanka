import type { FieldProps } from '../types';
import { FieldWrapper } from './FieldWrapper';

export function SelectField({ control, value, error, touched, onChange, onBlur, readOnly }: FieldProps) {
  const isReadonly = readOnly ?? control.options?.readonly;

  // Get options from enum or oneOf
  const options: { value: string | number; label: string }[] = [];

  if (control.property.oneOf) {
    for (const item of control.property.oneOf) {
      options.push({ value: item.const, label: item.title });
    }
  } else if (control.property.enum) {
    for (const item of control.property.enum) {
      options.push({ value: item, label: String(item) });
    }
  }

  return (
    <FieldWrapper control={control} error={error} touched={touched}>
      <select
        name={control.name}
        value={(value as string) ?? ''}
        onChange={(e) => onChange(e.target.value)}
        onBlur={onBlur}
        disabled={isReadonly}
        className={`
          w-full px-3 py-2 border rounded-md shadow-sm bg-white
          focus:outline-none focus:ring-2 focus:ring-blue-500 focus:border-blue-500
          disabled:bg-gray-100 disabled:cursor-not-allowed
          ${touched && error ? 'border-red-500' : 'border-gray-300'}
        `}
      >
        <option value="">
          {control.options?.placeholder ?? 'Select an option'}
        </option>
        {options.map((option) => (
          <option key={String(option.value)} value={option.value}>
            {option.label}
          </option>
        ))}
      </select>
    </FieldWrapper>
  );
}
