import type { ReactNode } from 'react'
import { Button, Text } from '@radix-ui/themes'
import { useTranslation } from 'react-i18next'

interface LoadErrorProps {
  message: string
  description?: string
  onRetry: () => void
  // A second action beside Try Again, such as going back to the list.
  secondary?: ReactNode
}

// A failed page load: the message, and a way to try the request again.
// An empty result is a different state and must not use this.
// A failed action uses the toast from ActionErrorProvider instead.
export function LoadError({ message, description, onRetry, secondary }: LoadErrorProps) {
  const { t } = useTranslation()
  return (
    <div className="flex flex-col items-center gap-4 text-center" role="alert">
      <div>
        <Text size="3" color="red" className="block">
          {message}
        </Text>
        {description && (
          <Text size="2" color="gray" className="mt-2 block">
            {description}
          </Text>
        )}
      </div>
      <div className="flex flex-wrap justify-center gap-3">
        {secondary}
        <Button type="button" variant="soft" onClick={onRetry}>
          {t('common.error.tryAgain')}
        </Button>
      </div>
    </div>
  )
}
