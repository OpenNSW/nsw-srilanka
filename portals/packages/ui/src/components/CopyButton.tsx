import { useState, type MouseEvent } from 'react'
import { IconButton, Tooltip } from '@radix-ui/themes'
import { CheckIcon, CopyIcon } from '@radix-ui/react-icons'
import { useUiLabels } from '../labels'

// An icon button that copies value, named "Copy <label>", and shows a check
// mark and says Copied for two seconds once it has. The clipboard API exists only on secure
// (HTTPS or localhost) pages; without it the button is not shown.
export function CopyButton({ value, label }: { value: string; label: string }) {
  const labels = useUiLabels()
  const [copied, setCopied] = useState(false)
  if (!navigator.clipboard) return null

  const copy = (e: MouseEvent) => {
    // A click on a tooltip's trigger closes the tooltip; keep it open so it
    // can say Copied.
    e.preventDefault()
    navigator.clipboard
      .writeText(value)
      .then(() => {
        setCopied(true)
        setTimeout(() => setCopied(false), 2000)
      })
      .catch((err) => console.error('CopyButton: failed to copy:', err))
  }
  const copyLabel = labels.copyNamed(label)
  return (
    <Tooltip content={copied ? labels.copied : copyLabel}>
      <IconButton type="button" size="1" variant="ghost" color="gray" aria-label={copyLabel} onClick={copy}>
        {copied ? <CheckIcon /> : <CopyIcon />}
      </IconButton>
    </Tooltip>
  )
}
