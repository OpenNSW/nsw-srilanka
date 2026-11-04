import type { ReactNode } from 'react'
import { Theme } from '@radix-ui/themes'
import { THEME_ROOT_PROPS } from '@portals/theme'

// Radix's root for the whole app. @portals/theme sets every --accent-* and
// --gray-* variable on each Theme root from the deployment's branding.json,
// outranking the scale Radix would pick; THEME_ROOT_PROPS are the props it
// needs on the root, including an inherited appearance, which follows the
// `dark` class AppearanceProvider puts on <html>. Components given their own
// color (color="red") keep that palette.
export function ThemedShell({ children }: { children: ReactNode }) {
  return (
    <Theme {...THEME_ROOT_PROPS} radius="large" scaling="105%" panelBackground="solid">
      {children}
    </Theme>
  )
}
