import type { HTMLAttributes } from 'react'
import { cx } from '../cx'

const ELEVATION = { sm: 'shadow-sm', md: 'shadow-md', xl: 'shadow-xl' } as const

interface SurfaceProps extends HTMLAttributes<HTMLElement> {
  // The element to render: a <details> keeps its own open and close.
  as?: 'div' | 'section' | 'details'
  elevation?: keyof typeof ELEVATION
}

// A panel raised off the page background. Padding, overflow and positioning
// are the caller's, through className.
export function Surface({ as: Element = 'div', elevation = 'md', className, ...rest }: SurfaceProps) {
  return <Element className={cx('rounded-2xl bg-app-surface', ELEVATION[elevation], className)} {...rest} />
}
