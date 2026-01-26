import type { FieldProps } from '../types';

export function CheckboxField({ control, value, error, touched, onChange, onBlur }: FieldProps) {
  const showError = touched && error;
  const isReadonly = control.options?.readonly;

  return (
    <div className="mb-4">
      <label className="flex items-start gap-2 cursor-pointer">
        <input
          type="checkbox"
          name={control.name}
          checked={(value as boolean) ?? false}
          onChange={(e) => onChange(e.target.checked)}
          onBlur={onBlur}
          disabled={isReadonly}
          className={`
            mt-1 h-4 w-4 rounded border-gray-300 text-blue-600
            focus:ring-2 focus:ring-blue-500
            disabled:cursor-not-allowed
          `}
        />
        <span className="text-sm text-gray-700">
          {control.label}
          {control.required && (
            <span className="text-red-500 ml-0.5">*</span>
          )}
        </span>
      </label>

      {control.property.description && !showError && (
        <p className="mt-1 ml-6 text-sm text-gray-500">{control.property.description}</p>
      )}

      {showError && (
        <p className="mt-1 ml-6 text-sm text-red-600">{error}</p>
      )}
    </div>
  );
}
