import type { ReactNode } from 'react'
import { Theme } from '@radix-ui/themes'

// Radix's root for the whole app. It names no accentColor or grayColor:
// @portals/theme sets every --accent-* and --gray-* variable on each Theme
// root from the deployment's branding.json, outranking the scale Radix would
// pick. Components given their own color (color="red") keep that palette.
export function ThemedShell({ children }: { children: ReactNode }) {
  return (
    <Theme radius="large" scaling="105%" panelBackground="solid" appearance="light">
      {children}
    </Theme>
  )
}
