import { useMemo, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { UiLabelsProvider, type UiLabels } from '@portals/ui'

// Gives @portals/ui's components their words in the current language.
export function TranslatedUiLabels({ children }: { children: ReactNode }) {
  const { t } = useTranslation()
  const labels = useMemo<UiLabels>(
    () => ({
      breadcrumb: t('common.breadcrumb'),
      copied: t('common.copied'),
      copyNamed: (name) => t('common.copyNamed', { name }),
      tryAgain: t('common.error.tryAgain'),
      pagination: {
        total: (count) => t('common.pagination.total', { count }),
        page: (page, totalPages) => t('common.pagination.page', { page, totalPages }),
        previous: t('common.pagination.previous'),
        next: t('common.pagination.next'),
      },
    }),
    [t],
  )
  return <UiLabelsProvider labels={labels}>{children}</UiLabelsProvider>
}
