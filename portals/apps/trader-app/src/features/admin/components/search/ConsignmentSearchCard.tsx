import { useId, useState, type ChangeEvent, type FormEvent } from 'react'
import { Button, Callout, Text, TextField } from '@radix-ui/themes'
import { ExclamationTriangleIcon, MagnifyingGlassIcon } from '@radix-ui/react-icons'

// The wording for one mode (TNSW consignments or agency cases). The screen picks it, so the
// card has no mode logic of its own.
export interface SearchCopy {
  title: string
  subtitle: string
  idLabel: string
  helper: string
  notFound: string
  error: string
}

export type SearchStatus = 'idle' | 'notFound' | 'error'

interface ConsignmentSearchCardProps {
  copy: SearchCopy
  onSearch: (id: string) => void
  pending?: boolean
  status?: SearchStatus
  onRetry?: () => void
  // Called when the ID is edited, so the screen can clear a not-found message.
  onChange?: () => void
}

// A single exact-match ID lookup. Admins can't browse consignments or cases: every lookup
// starts from an ID someone gave them.
export function ConsignmentSearchCard({
  copy,
  onSearch,
  pending = false,
  status = 'idle',
  onRetry,
  onChange,
}: ConsignmentSearchCardProps) {
  const [value, setValue] = useState('')
  const inputId = useId()
  const helperId = useId()
  const trimmed = value.trim()

  function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    if (!trimmed || pending) return
    onSearch(trimmed)
  }

  function handleChange(event: ChangeEvent<HTMLInputElement>) {
    setValue(event.target.value)
    onChange?.()
  }

  return (
    <div className="bg-app-surface rounded-lg shadow p-5">
      <form onSubmit={handleSubmit} noValidate>
        <label htmlFor={inputId} className="mb-2 block text-sm font-medium text-foreground">
          {copy.idLabel}
        </label>
        <div className="flex flex-col gap-2 sm:flex-row">
          <div className="flex-1">
            <TextField.Root
              id={inputId}
              size="3"
              className="font-mono"
              placeholder="e.g. 0b6f3c2e-…"
              autoFocus
              autoComplete="off"
              spellCheck={false}
              value={value}
              onChange={handleChange}
              readOnly={pending}
              color={status === 'notFound' ? 'red' : undefined}
              aria-invalid={status === 'notFound' || undefined}
              aria-describedby={helperId}
            />
          </div>
          <Button type="submit" size="3" className="w-full sm:w-auto" disabled={!trimmed} loading={pending}>
            <MagnifyingGlassIcon />
            {pending ? 'Searching…' : 'Search'}
          </Button>
        </div>

        <div aria-live="polite">
          {status === 'notFound' && (
            <Text as="p" size="2" color="red" role="alert" className="mt-2 flex items-center gap-1">
              <ExclamationTriangleIcon />
              {copy.notFound}
            </Text>
          )}
        </div>

        <Text as="p" id={helperId} size="2" className="mt-2 text-foreground-muted">
          {copy.helper}
        </Text>
      </form>

      {status === 'error' && (
        <Callout.Root color="red" role="alert" className="mt-4">
          <Callout.Icon>
            <ExclamationTriangleIcon />
          </Callout.Icon>
          <div className="flex flex-wrap items-center justify-between gap-2">
            <Callout.Text>{copy.error}</Callout.Text>
            {onRetry && (
              <Button type="button" variant="outline" color="gray" size="1" onClick={onRetry}>
                Retry
              </Button>
            )}
          </div>
        </Callout.Root>
      )}
    </div>
  )
}
