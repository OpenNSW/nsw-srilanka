import { useState } from 'react'
import { IconButton, Tooltip } from '@radix-ui/themes'
import { CheckIcon, CopyIcon } from '@radix-ui/react-icons'
import { useTranslation } from 'react-i18next'

// A labelled ID with a copy button, for the IDs traders and support quote.
export function CopyableId({ label, id }: { label: string; id: string }) {
  const { t } = useTranslation()
  const [copied, setCopied] = useState(false)
  const copy = () => {
    void navigator.clipboard.writeText(id).then(() => {
      setCopied(true)
      setTimeout(() => setCopied(false), 2000)
    })
  }
  const copyLabel = t('common.copyNamed', { name: label })
  return (
    <span className="inline-flex items-center gap-1 text-xs text-foreground-muted">
      <span>{label}</span>
      <span className="font-mono">{id}</span>
      <Tooltip content={copied ? t('common.copied') : copyLabel}>
        <IconButton type="button" size="1" variant="ghost" color="gray" aria-label={copyLabel} onClick={copy}>
          {copied ? <CheckIcon /> : <CopyIcon />}
        </IconButton>
      </Tooltip>
    </span>
  )
}
