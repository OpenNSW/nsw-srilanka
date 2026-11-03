import { Button } from '@radix-ui/themes'
import { Translation } from 'react-i18next'
import { Surface, PageState } from '@portals/ui'

// The whole-page fallback for the root ErrorBoundary. It renders outside every
// provider, so it reads translations through <Translation>, not a hook, and
// gives its own Try Again button rather than PageState's onRetry, whose label
// comes from a provider it is outside of.
export function ErrorFallback({ error, onReset }: { error: Error; onReset: () => void }) {
  return (
    <Translation>
      {(t) => (
        <div className="min-h-screen flex items-center justify-center bg-app-bg p-4">
          <Surface elevation="xl" className="max-w-md w-full">
            <PageState
              variant="error"
              title={t('common.error.title')}
              description={error.message || t('common.error.unexpected')}
              actions={
                <Button onClick={onReset} size="3">
                  {t('common.error.tryAgain')}
                </Button>
              }
            />
          </Surface>
        </div>
      )}
    </Translation>
  )
}
