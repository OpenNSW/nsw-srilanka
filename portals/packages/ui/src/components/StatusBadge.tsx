import type { ReactNode } from 'react'
import { Badge } from '@radix-ui/themes'

// What a status means, not its color: each feature maps its own statuses to a
// tone, so the same meaning is the same color on every screen.
const TONE_COLOR = {
  neutral: 'gray',
  info: 'blue',
  success: 'green',
  warning: 'amber',
  danger: 'red',
} as const

export type StatusTone = keyof typeof TONE_COLOR

interface StatusBadgeProps {
  tone: StatusTone
  children: ReactNode
  // 1 in a list row or table cell; 2 beside a page or card title.
  size?: '1' | '2'
  title?: string
}

export function StatusBadge({ tone, children, size = '1', title }: StatusBadgeProps) {
  return (
    <Badge variant="soft" size={size} color={TONE_COLOR[tone]} title={title}>
      {children}
    </Badge>
  )
}
