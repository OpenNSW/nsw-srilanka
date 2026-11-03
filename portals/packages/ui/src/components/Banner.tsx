import type { ReactNode } from 'react'
import { CheckCircledIcon, ExclamationTriangleIcon, InfoCircledIcon } from '@radix-ui/react-icons'
import { cx } from '../cx'

export type BannerVariant = 'info' | 'success' | 'warning' | 'error'

const STYLES: Record<BannerVariant, { container: string; text: string }> = {
  info: { container: 'bg-info-subtle', text: 'text-info-strong' },
  success: { container: 'bg-success-subtle', text: 'text-success-strong' },
  warning: { container: 'bg-warning-subtle', text: 'text-warning-strong' },
  error: { container: 'bg-error-subtle', text: 'text-error-strong' },
}

const ICONS: Record<BannerVariant, typeof InfoCircledIcon> = {
  info: InfoCircledIcon,
  success: CheckCircledIcon,
  warning: ExclamationTriangleIcon,
  error: ExclamationTriangleIcon,
}

interface BannerProps {
  variant?: BannerVariant
  title?: string
  // The message. Line breaks in a string are kept.
  children: ReactNode
  className?: string
}

// A message about the page or what just happened on it. An error is announced
// to screen readers as it appears.
export function Banner({ variant = 'info', title, children, className }: BannerProps) {
  const { container, text } = STYLES[variant]
  const Icon = ICONS[variant]
  return (
    <div
      role={variant === 'error' ? 'alert' : undefined}
      className={cx('flex items-start gap-3 rounded-2xl p-4', container, text, className)}
    >
      <Icon aria-hidden className="mt-0.5 size-4 shrink-0" />
      <div className="min-w-0 flex-1 text-sm">
        {title && <p className="font-semibold">{title}</p>}
        <div className="whitespace-pre-wrap">{children}</div>
      </div>
    </div>
  )
}
