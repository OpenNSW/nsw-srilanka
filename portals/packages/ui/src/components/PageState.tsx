import type { ReactNode } from 'react'
import { Button, Spinner, Text } from '@radix-ui/themes'
import { useUiLabels } from '../labels'
import { cx } from '../cx'

type PageStateProps =
  // Nothing to show yet; the title says what is loading.
  | { variant: 'loading'; title: string; className?: string }
  // Nothing to show, and nothing went wrong: no rows, no workflow yet.
  | { variant: 'empty'; title: string; description?: string; actions?: ReactNode; className?: string }
  // A load that failed. onRetry adds a Try Again button after any actions.
  | {
      variant: 'error'
      title: string
      description?: string
      actions?: ReactNode
      onRetry?: () => void
      className?: string
    }

// What a page or panel shows in place of its content: loading, empty, or
// failed to load. Content being reloaded in place uses LoadingOverlay instead.
export function PageState(props: PageStateProps) {
  const labels = useUiLabels()
  const { variant, title, className } = props
  const description = variant === 'loading' ? undefined : props.description
  const actions = variant === 'loading' ? undefined : props.actions
  const onRetry = variant === 'error' ? props.onRetry : undefined

  return (
    <div
      role={variant === 'loading' ? 'status' : undefined}
      className={cx('flex flex-col items-center gap-2 px-6 py-12 text-center', className)}
    >
      {variant === 'loading' && <Spinner size="3" />}
      <Text
        size={variant === 'error' ? '5' : '3'}
        weight={variant === 'loading' ? 'regular' : 'medium'}
        color={variant === 'error' ? 'red' : 'gray'}
      >
        {title}
      </Text>
      {description && (
        <Text size="2" color="gray">
          {description}
        </Text>
      )}
      {(actions || onRetry) && (
        <div className="mt-4 flex flex-wrap justify-center gap-3">
          {actions}
          {onRetry && (
            <Button variant="soft" onClick={onRetry}>
              {labels.tryAgain}
            </Button>
          )}
        </div>
      )}
    </div>
  )
}
