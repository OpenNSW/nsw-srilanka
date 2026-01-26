import type { ReactNode } from 'react';
import type { ResolvedControl } from '../types';

interface FieldWrapperProps {
  control: ResolvedControl;
  error?: string;
  touched: boolean;
  children: ReactNode;
}

export function FieldWrapper({ control, error, touched, children }: FieldWrapperProps) {
  const showError = touched && error;

  return (
    <div className="mb-4">
      <label className="block mb-1.5">
        <span className="text-sm font-medium text-gray-700">
          {control.label}
          {control.required && (
            <span className="text-red-500 ml-0.5">*</span>
          )}
        </span>
      </label>

      {children}

      {control.property.description && !showError && (
        <p className="mt-1 text-sm text-gray-500">{control.property.description}</p>
      )}

      {showError && (
        <p className="mt-1 text-sm text-red-600">{error}</p>
      )}
    </div>
  );
}
