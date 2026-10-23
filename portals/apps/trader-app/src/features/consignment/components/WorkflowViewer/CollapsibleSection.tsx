import React, { useId, useState } from 'react'
import { Badge, Box } from '@radix-ui/themes'
import { ChevronDownIcon } from '@radix-ui/react-icons'

export interface CollapsibleSectionProps {
  title: string
  count: number
  children: React.ReactNode
  defaultOpen?: boolean
  color?: 'gray' | 'blue' | 'green'
}

// A section heading that opens and closes its list. The whole heading row is
// the button, so its name is the visible text ("Process History 2") and it
// says whether it's open.
export const CollapsibleSection = ({
  title,
  count,
  children,
  defaultOpen = false,
  color = 'gray' as const,
}: CollapsibleSectionProps) => {
  const [isOpen, setIsOpen] = useState(defaultOpen)
  const contentId = useId()

  if (count === 0) return null

  return (
    <Box mt="5">
      <h2 className="mb-3">
        <button
          type="button"
          aria-expanded={isOpen}
          aria-controls={contentId}
          onClick={() => setIsOpen(!isOpen)}
          className="flex w-full items-center justify-between rounded-lg px-3 py-2 text-left transition-colors hover:bg-app-surface-muted"
        >
          <span className="flex items-center gap-2">
            <span
              className={`text-base font-bold ${color === 'green' ? 'text-success-strong' : color === 'blue' ? 'text-info-strong' : 'text-foreground'}`}
            >
              {title}
            </span>
            <Badge color={color} variant="soft" radius="full">
              {count}
            </Badge>
          </span>
          <ChevronDownIcon
            aria-hidden
            width="20"
            height="20"
            className={`text-foreground-muted transition-transform ${isOpen ? 'rotate-180' : ''}`}
          />
        </button>
      </h2>
      <Box id={contentId} px="1" hidden={!isOpen}>
        {isOpen && children}
      </Box>
    </Box>
  )
}
