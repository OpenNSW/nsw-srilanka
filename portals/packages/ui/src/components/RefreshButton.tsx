import { IconButton, Tooltip } from '@radix-ui/themes'
import { ReloadIcon } from '@radix-ui/react-icons'

interface RefreshButtonProps {
  // The tooltip and the accessible name.
  label: string
  // Spins the icon and disables the button until the reload ends.
  refreshing: boolean
  onClick: () => void
}

// Reloads what the page shows.
export function RefreshButton({ label, refreshing, onClick }: RefreshButtonProps) {
  return (
    <Tooltip content={label}>
      <IconButton type="button" variant="soft" color="gray" onClick={onClick} disabled={refreshing} aria-label={label}>
        <ReloadIcon className={refreshing ? 'animate-spin' : ''} />
      </IconButton>
    </Tooltip>
  )
}
