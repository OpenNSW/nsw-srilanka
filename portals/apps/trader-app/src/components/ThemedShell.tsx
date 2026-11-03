import type { ReactNode } from 'react'
import { Theme } from '@radix-ui/themes'

// Radix's root for the whole app. @portals/theme sets every --accent-* and
// --gray-* variable on each Theme root from the deployment's branding.json,
// outranking the scale Radix would pick. grayColor="gray" is the one value
// Radix doesn't remap, so a theme whose gray scale is gray keeps it (the
// default, auto, would swap in slate). Components given their own color
// (color="red") keep that palette.
export function ThemedShell({ children }: { children: ReactNode }) {
  return (
    <Theme grayColor="gray" radius="large" scaling="105%" panelBackground="solid" appearance="light">
      {children}
    </Theme>
  )
}
