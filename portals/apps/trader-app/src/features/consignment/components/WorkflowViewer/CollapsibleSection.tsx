import React, { useState } from 'react'
import { Badge, Flex, Box, Heading, IconButton } from '@radix-ui/themes'
import { ChevronDownIcon, ChevronUpIcon } from '@radix-ui/react-icons'

export interface CollapsibleSectionProps {
  title: string
  count: number
  children: React.ReactNode
  defaultOpen?: boolean
  color?: 'gray' | 'blue' | 'green'
}

export const CollapsibleSection = ({
  title,
  count,
  children,
  defaultOpen = false,
  color = 'gray' as const,
}: CollapsibleSectionProps) => {
  const [isOpen, setIsOpen] = useState(defaultOpen)

  if (count === 0) return null

  return (
    <Box mt="5">
      <Flex
        align="center"
        justify="between"
        className="cursor-pointer py-2 px-3 hover:bg-background rounded-lg transition-colors border-b border-border mb-3"
        onClick={() => setIsOpen(!isOpen)}
      >
        <Flex align="center" gap="2">
          <Heading size="3" color={color} weight="bold">
            {title}
          </Heading>
          <Badge color={color} variant="soft" radius="full">
            {count}
          </Badge>
        </Flex>
        <IconButton variant="ghost" color="gray" size="1">
          {isOpen ? <ChevronUpIcon /> : <ChevronDownIcon />}
        </IconButton>
      </Flex>
      {isOpen && <Box px="1">{children}</Box>}
    </Box>
  )
}
