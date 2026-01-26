import type { FieldProps } from '../types';
import { FieldWrapper } from './FieldWrapper';

export function NumberField({ control, value, error, touched, onChange, onBlur }: FieldProps) {
  const isReadonly = control.options?.readonly;

  const handleChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    const inputValue = e.target.value;

    if (inputValue === '') {
      onChange('');
      return;
    }

    const numValue = parseFloat(inputValue);
    if (!isNaN(numValue)) {
      onChange(numValue);
    }
  };

  return (
    <FieldWrapper control={control} error={error} touched={touched}>
      <input
        type="number"
        name={control.name}
        value={value as string | number ?? ''}
        onChange={handleChange}
        onBlur={onBlur}
        placeholder={control.options?.placeholder}
        disabled={isReadonly}
        min={control.property.minimum}
        max={control.property.maximum}
        step={control.property.multipleOf ?? (control.property.type === 'integer' ? 1 : 'any')}
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
