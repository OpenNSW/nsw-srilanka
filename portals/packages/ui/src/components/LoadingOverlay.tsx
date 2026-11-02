import { Spinner, Text } from '@radix-ui/themes'

// Covers content that is being reloaded, which stays visible underneath. The
// parent must be positioned (relative); the overlay takes its rounded corners.
export function LoadingOverlay({ message }: { message: string }) {
  return (
    <div
      role="status"
      className="absolute inset-0 z-10 flex flex-col items-center justify-center gap-2 rounded-[inherit] bg-app-surface/60 backdrop-blur-[1px]"
    >
      <Spinner size="3" />
      <Text size="2" color="gray">
        {message}
      </Text>
    </div>
  )
}
