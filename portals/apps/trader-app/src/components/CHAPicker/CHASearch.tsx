import { useState } from 'react'
import { Text, Box, Flex, Spinner, TextField, ScrollArea, IconButton, Badge } from '@radix-ui/themes'
import { MagnifyingGlassIcon, Cross2Icon } from '@radix-ui/react-icons'

export type CHAOption = {
  id: string
  name: string
}

interface CHASearchProps {
  value: CHAOption | null
  onChange: (cha: CHAOption | null) => void
  // Server-filtered results for the current searchQuery. The parent is responsible
  // for refetching this list (debounced) whenever searchQuery changes.
  options: readonly CHAOption[]
  searchQuery: string
  onSearchQueryChange: (query: string) => void
  loading?: boolean
}

export function CHASearch({
  value,
  onChange,
  options,
  searchQuery,
  onSearchQueryChange,
  loading = false,
}: CHASearchProps) {
  const [isFocused, setIsFocused] = useState(false)

  const handleSelect = (cha: CHAOption) => {
    onSearchQueryChange(cha.name)
    onChange(cha)
    setIsFocused(false)
  }

  const handleClear = () => {
    onSearchQueryChange('')
    onChange(null)
  }

  const showDropdown = isFocused && searchQuery.length > 0

  return (
    <Box position="relative">
      <TextField.Root
        size="2"
        placeholder="Search company (e.g., ADAM, EDWARD)..."
        value={searchQuery}
        onChange={(e) => {
          const val = e.target.value
          onSearchQueryChange(val)
          setIsFocused(true)
          if (value && val !== value.name) {
            onChange(null)
          }
        }}
        onFocus={() => setIsFocused(true)}
        onBlur={() => setTimeout(() => setIsFocused(false), 150)}
        onMouseDown={() => setIsFocused(true)}
      >
        <TextField.Slot>
          <MagnifyingGlassIcon height="16" width="16" />
        </TextField.Slot>
        {loading && (
          <TextField.Slot>
            <Spinner size="1" />
          </TextField.Slot>
        )}
        {searchQuery && (
          <TextField.Slot>
            <IconButton size="1" variant="ghost" onClick={handleClear}>
              <Cross2Icon height="14" width="14" />
            </IconButton>
          </TextField.Slot>
        )}
      </TextField.Root>

      {showDropdown && (
        <Box
          position="absolute"
          width="100%"
          mt="1"
          className="bg-background border border-border rounded-md shadow-lg z-10 overflow-hidden"
        >
          {options.length === 0 ? (
            <Flex align="center" justify="center" py="5" direction="column" gap="1">
              <Text size="2" color="gray">
                {loading ? 'Searching…' : `No companies found for "${searchQuery}"`}
              </Text>
            </Flex>
          ) : (
            <ScrollArea style={{ maxHeight: '280px' }}>
              <Box py="1">
                {options.map((company) => {
                  const isSelected = value?.id === company.id
                  return (
                    <Flex
                      key={company.id}
                      px="3"
                      py="2"
                      gap="3"
                      align="start"
                      className={`cursor-pointer transition-colors ${
                        isSelected ? 'bg-info-subtle hover:bg-info/15' : 'hover:bg-surface'
                      }`}
                      onMouseDown={(e) => {
                        e.preventDefault()
                        handleSelect(company)
                      }}
                    >
                      <Box pt="1">
                        <MagnifyingGlassIcon height="14" width="14" className="text-foreground-subtle" />
                      </Box>
                      <Box style={{ flex: 1, minWidth: 0 }}>
                        <Flex align="center" gap="2" mb="1">
                          <Text size="2" weight="medium">
                            {company.name}
                          </Text>
                          <Badge size="1" color="blue" variant="soft">
                            CHA Company
                          </Badge>
                        </Flex>
                      </Box>
                    </Flex>
                  )
                })}
              </Box>
            </ScrollArea>
          )}
        </Box>
      )}
    </Box>
  )
}
