import { useState } from 'react'
import { IconButton, Tooltip } from '@radix-ui/themes'
import { CheckIcon, CopyIcon } from '@radix-ui/react-icons'
import { useUiLabels } from '../labels'

// A labelled ID with a copy button, for the IDs traders and support quote.
export function CopyableId({ label, id }: { label: string; id: string }) {
  const labels = useUiLabels()
  const [copied, setCopied] = useState(false)
  // The clipboard API exists only on secure (HTTPS or localhost) pages; without
  // it the ID is shown with no copy button.
  const canCopy = !!navigator.clipboard
  const copy = () => {
    navigator.clipboard
      .writeText(id)
      .then(() => {
        setCopied(true)
        setTimeout(() => setCopied(false), 2000)
      })
      .catch((err) => console.error('CopyableId: failed to copy:', err))
  }
  const copyLabel = labels.copyNamed(label)
  // The ID breaks across lines rather than overflow a narrow screen, so all of
  // it stays readable.
  return (
    <span className="inline-flex max-w-full items-center gap-1 text-xs text-foreground-muted">
      <span className="shrink-0">{label}</span>
      <span className="min-w-0 break-all font-mono">{id}</span>
      {canCopy && (
        <Tooltip content={copied ? labels.copied : copyLabel}>
          <IconButton type="button" size="1" variant="ghost" color="gray" aria-label={copyLabel} onClick={copy}>
            {copied ? <CheckIcon /> : <CopyIcon />}
          </IconButton>
        </Tooltip>
      )}
    </span>
  )
}
