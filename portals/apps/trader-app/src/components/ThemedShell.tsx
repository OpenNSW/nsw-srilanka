import type { ReactNode } from 'react'
import { Theme } from '@radix-ui/themes'
import { useColorScheme } from '@/services/useColorScheme'

// Reads the active color scheme so <Theme accentColor> stays in sync with
// the CSS variable overrides the scheme applies (see ColorSchemeContext +
// the `[data-theme]` blocks in index.css). Split out from main.tsx because
// it needs to be inside ColorSchemeProvider to read the context, while
// <Theme> itself needs to wrap everything else in the app.
export function ThemedShell({ children }: { children: ReactNode }) {
  const { scheme } = useColorScheme()

  return (
    <Theme
      accentColor={scheme.accentColor}
      grayColor="slate"
      radius="large"
      scaling="105%"
      panelBackground="solid"
      appearance="light"
    >
      {children}
    </Theme>
  )
}
