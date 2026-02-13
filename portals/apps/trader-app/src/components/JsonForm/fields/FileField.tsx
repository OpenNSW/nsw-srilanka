import type { FieldProps } from '../types';
import { FieldWrapper } from './FieldWrapper';
import { useState, useEffect, useRef } from 'react';
import { getFileUrl } from '../../../services/upload';
import { UploadIcon, ExclamationTriangleIcon } from '@radix-ui/react-icons';

export function FileField({ control, value, error, touched, onChange, onBlur, readOnly }: FieldProps) {
  const isReadonly = readOnly ?? control.options?.readonly;
  const [displayName, setDisplayName] = useState<string>('');
  const [previewUrl, setPreviewUrl] = useState<string>('');
  const [dragActive, setDragActive] = useState(false);
  const [validationError, setValidationError] = useState<string | null>(null);
  const inputRef = useRef<HTMLInputElement>(null);

  // Get options from control (or defaults)
  const maxSize = (control.options?.maxSize as number) || 5 * 1024 * 1024; // Default 5MB
  const accept = (control.options?.accept as string) || '*/*';

  useEffect(() => {
    if (value instanceof File) {
      setDisplayName(value.name);
      const objectUrl = URL.createObjectURL(value);
      setPreviewUrl(objectUrl);
      return () => URL.revokeObjectURL(objectUrl);
    }

    if (typeof value === 'string' && value) {
      const parts = value.split('/');
      setDisplayName(parts[parts.length - 1] || value);
      setPreviewUrl(getFileUrl(value));
      return undefined;
    }

    setDisplayName('');
    setPreviewUrl('');
    return undefined;
  }, [value]);

  const validateAndProcessFile = (file: File) => {
    if (!file) return;

    // Validate file size
    if (file.size > maxSize) {
      const sizeMB = (maxSize / (1024 * 1024)).toFixed(0);
      setValidationError(`File size exceeds ${sizeMB}MB limit.`);
      return;
    }

    // Validate file type
    const acceptedTypes = accept.split(',').map((t: string) => t.trim());
    const isFileTypeAccepted = acceptedTypes.some((type: string) => {
      if (type.endsWith('/*')) {
        return file.type.startsWith(type.slice(0, -1));
      }
      if (type.startsWith('.')) {
        return file.name.toLowerCase().endsWith(type.toLowerCase());
      }
      return file.type === type;
    });

    if (accept !== '*/*' && !isFileTypeAccepted && !accept.includes('*/*')) {
      setValidationError(`Invalid file type. Accepted types: ${accept}`);
      return;
    }

    // File is valid
    setValidationError(null);
    onChange(file);
    setDisplayName(file.name);
  };

  const handleInputChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0];
    if (file) validateAndProcessFile(file);
  };

  const handleDrag = (e: React.DragEvent<HTMLDivElement>) => {
    e.preventDefault();
    e.stopPropagation();
    if (isReadonly || displayName) return;

    if (e.type === 'dragenter' || e.type === 'dragover') {
      setDragActive(true);
    } else if (e.type === 'dragleave') {
      setDragActive(false);
    }
  };

  const handleDrop = (e: React.DragEvent<HTMLDivElement>) => {
    e.preventDefault();
    e.stopPropagation();
    setDragActive(false);
    if (isReadonly || displayName) return;

    const file = e.dataTransfer.files?.[0];
    if (file) validateAndProcessFile(file);
  };

  const handleRemove = () => {
    onChange(null);
    setDisplayName('');
    setPreviewUrl('');
    setValidationError(null);
    if (inputRef.current) inputRef.current.value = '';
  };

  const showFileInfo = Boolean(displayName);
  const hasPreview = Boolean(previewUrl);
  const hasError = Boolean(validationError);

  // Show remove only if it is not readonly AND the value is a File object (meaning it's a new upload).
  // If value is a string, it implies it is a saved file from the server (Completed state), so we hide Remove.
  const showRemoveButton = !isReadonly ;

  return (
    <FieldWrapper control={control} error={error} touched={touched}>
      <div className="space-y-3">
        {!showFileInfo ? (
          <div
            className={`
              border rounded-xl py-10 px-6 text-center transition-all duration-200 ease-in-out
              ${dragActive ? 'border-blue-500 bg-blue-50' : hasError ? 'border-red-200 bg-red-50' : 'border-gray-200 bg-slate-50/50 hover:border-blue-400 hover:bg-slate-50'}
              ${touched && error ? 'border-red-200 bg-red-50' : ''}
              ${isReadonly ? 'opacity-60 cursor-not-allowed' : 'cursor-pointer'}
            `}
            onDragEnter={handleDrag}
            onDragLeave={handleDrag}
            onDragOver={handleDrag}
            onDrop={handleDrop}
            onClick={() => !isReadonly && inputRef.current?.click()}
            role="button"
            tabIndex={isReadonly ? -1 : 0}
          >
            <input
              ref={inputRef}
              type="file"
              name={control.name}
              onChange={handleInputChange}
              onBlur={onBlur}
              disabled={isReadonly}
              accept={accept}
              className="hidden"
            />

            <div className="flex flex-col items-center gap-3">
              {hasError ? (
                <>
                  <ExclamationTriangleIcon className="w-8 h-8 text-red-500" />
                  <p className="text-base font-medium text-red-600">
                    {validationError}
                  </p>
                  <p className="text-sm text-gray-500">Click to try again</p>
                </>
              ) : (
                <>
                  <UploadIcon className="w-8 h-8 text-gray-400 mb-1" />
                  <p className="text-base font-medium text-gray-600">
                    Click to upload or drag and drop
                  </p>
                  <p className="text-sm text-gray-500 opacity-70">
                    Max {Math.round(maxSize / (1024 * 1024))}MB
                  </p>
                </>
              )}
            </div>
          </div>
        ) : (
          <div className="rounded-lg border border-gray-200 bg-white p-4">
            <div className="flex items-center justify-between gap-3">
              <div className="flex-1 min-w-0">
                <p className="text-sm font-medium text-gray-900 truncate">{displayName}</p>
              </div>

              <div className="flex items-center gap-2">
                {hasPreview && (
                  <a
                    href={previewUrl}
                    target="_blank"
                    rel="noopener noreferrer"
                    className="inline-flex items-center justify-center text-sm font-medium text-blue-700 bg-blue-50 px-3 py-1.5 rounded-md border border-blue-200 hover:bg-blue-100"
                  >
                    View
                  </a>
                )}

                {showRemoveButton && (
                  <button
                    type="button"
                    onClick={handleRemove}
                    className="inline-flex items-center justify-center text-sm font-medium text-red-700 bg-red-50 px-3 py-1.5 rounded-md border border-red-200 hover:bg-red-100"
                  >
                    Remove
                  </button>
                )}
              </div>
            </div>
          </div>
        )}
      </div>
    </FieldWrapper>
  );
}