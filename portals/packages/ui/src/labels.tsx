import { createContext, useContext, type ReactNode } from 'react'

// The fixed words the components show. The package does no translation: an
// app passes its own translated labels once through UiLabelsProvider.
export interface UiLabels {
  breadcrumb: string
  copied: string
  copyNamed: (name: string) => string
  tryAgain: string
  pagination: {
    total: (count: number) => string
    page: (page: number, totalPages: number) => string
    previous: string
    next: string
  }
}

// English, for an app that sets no provider, and for tests.
export const defaultUiLabels: UiLabels = {
  breadcrumb: 'Breadcrumb',
  copied: 'Copied',
  copyNamed: (name) => `Copy ${name}`,
  tryAgain: 'Try Again',
  pagination: {
    total: (count) => `Total: ${count}`,
    page: (page, totalPages) => `Page ${page} of ${totalPages}`,
    previous: 'Previous',
    next: 'Next',
  },
}

const UiLabelsContext = createContext<UiLabels>(defaultUiLabels)

export function UiLabelsProvider({ labels, children }: { labels: UiLabels; children: ReactNode }) {
  return <UiLabelsContext value={labels}>{children}</UiLabelsContext>
}

export function useUiLabels(): UiLabels {
  return useContext(UiLabelsContext)
}
