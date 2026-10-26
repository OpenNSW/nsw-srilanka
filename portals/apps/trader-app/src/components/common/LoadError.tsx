import { Button, Text } from '@radix-ui/themes'
import { useTranslation } from 'react-i18next'

interface LoadErrorProps {
  message: string
  onRetry: () => void
}

// A failed page load: the message, and a way to try the request again.
// An empty result is a different state and must not use this.
export function LoadError({ message, onRetry }: LoadErrorProps) {
  const { t } = useTranslation()
  return (
    <div className="flex flex-col items-center gap-4 text-center">
      <Text size="3" color="red">
        {message}
      </Text>
      <Button variant="soft" onClick={onRetry}>
        {t('common.error.tryAgain')}
      </Button>
    </div>
  )
}
