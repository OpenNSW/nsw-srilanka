import { Button, IconButton, Tooltip } from '@radix-ui/themes'
import { ReloadIcon } from '@radix-ui/react-icons'

interface RefreshButtonProps {
  // The tooltip and the accessible name, or the button's text with showLabel.
  label: string
  // Spins the icon and disables the button until the reload ends.
  refreshing: boolean
  onClick: () => void
  // Shows the label beside the icon, for a toolbar of text buttons, instead
  // of an icon button with a tooltip.
  showLabel?: boolean
}

// Reloads what the page shows.
export function RefreshButton({ label, refreshing, onClick, showLabel = false }: RefreshButtonProps) {
  const icon = <ReloadIcon className={refreshing ? 'animate-spin' : ''} />
  if (showLabel) {
    return (
      <Button type="button" variant="soft" color="gray" onClick={onClick} disabled={refreshing}>
        {icon}
        {label}
      </Button>
    )
  }
  return (
    <Tooltip content={label}>
      <IconButton type="button" variant="soft" color="gray" onClick={onClick} disabled={refreshing} aria-label={label}>
        {icon}
      </IconButton>
    </Tooltip>
  )
}
