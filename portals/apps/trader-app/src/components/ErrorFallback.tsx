import { Button, Text } from '@radix-ui/themes'
import { Translation } from 'react-i18next'

// The whole-page fallback for the root ErrorBoundary. It renders outside every
// provider, so it reads translations through <Translation>, not a hook.
export function ErrorFallback({ error, onReset }: { error: Error; onReset: () => void }) {
  return (
    <Translation>
      {(t) => (
        <div className="min-h-screen flex items-center justify-center bg-app-bg p-4">
          <div className="bg-app-surface rounded-2xl shadow-xl p-8 max-w-md w-full text-center">
            <div className="mb-4">
              <Text size="6" weight="bold" className="text-error-strong">
                {t('common.error.title')}
              </Text>
            </div>
            <div className="mb-6">
              <Text size="3" color="gray">
                {error.message || t('common.error.unexpected')}
              </Text>
            </div>
            <Button onClick={onReset} size="3">
              {t('common.error.tryAgain')}
            </Button>
          </div>
        </div>
      )}
    </Translation>
  )
}
