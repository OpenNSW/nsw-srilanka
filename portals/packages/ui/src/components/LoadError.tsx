import { Button, Text } from '@radix-ui/themes'
import { useUiLabels } from '../labels'

interface LoadErrorProps {
  message: string
  onRetry: () => void
}

// A failed page load: the message, and a way to try the request again.
// An empty result is a different state and must not use this.
export function LoadError({ message, onRetry }: LoadErrorProps) {
  const labels = useUiLabels()
  return (
    <div className="flex flex-col items-center gap-4 text-center">
      <Text size="3" color="red">
        {message}
      </Text>
      <Button variant="soft" onClick={onRetry}>
        {labels.tryAgain}
      </Button>
    </div>
  )
}
