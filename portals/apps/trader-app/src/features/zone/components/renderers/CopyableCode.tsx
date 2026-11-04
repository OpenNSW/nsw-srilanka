import { Children, useState, type ReactNode } from 'react'
import { IconButton, Tooltip } from '@radix-ui/themes'
import { CheckIcon, CopyIcon } from '@radix-ui/react-icons'
import { useUiLabels } from '@portals/ui'

// An inline code span with a copy button, for the values a template asks the
// trader to quote elsewhere, such as a payment or agency reference number.
export function CopyableCode({ children }: { children?: ReactNode }) {
  const labels = useUiLabels()
  const [copied, setCopied] = useState(false)
  const text = Children.toArray(children)
    .filter((child) => typeof child === 'string')
    .join('')
  // The clipboard API exists only on secure (HTTPS or localhost) pages; without
  // it the value is shown with no copy button.
  const canCopy = !!navigator.clipboard
  const copy = () => {
    navigator.clipboard
      .writeText(text)
      .then(() => {
        setCopied(true)
        setTimeout(() => setCopied(false), 2000)
      })
      .catch((err) => console.error('CopyableCode: failed to copy:', err))
  }
  const copyLabel = labels.copyNamed(text)
  return (
    <span className="inline-flex items-center gap-1 align-middle">
      <code className="bg-app-surface-muted text-primary-strong px-1.5 py-0.5 rounded text-xs font-mono">{text}</code>
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
