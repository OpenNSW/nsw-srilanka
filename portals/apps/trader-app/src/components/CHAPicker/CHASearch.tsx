import { useMemo, useState } from 'react'
import { Text, Box, Flex, Spinner, TextField, ScrollArea, IconButton, Badge } from '@radix-ui/themes'
import { MagnifyingGlassIcon, Cross2Icon } from '@radix-ui/react-icons'

export type CHAOption = {
  id: string
  name: string
}

interface CHASearchProps {
  value: CHAOption | null
  onChange: (cha: CHAOption | null) => void
  options: readonly CHAOption[]
  searchQuery: string
  onSearchQueryChange: (query: string) => void
}

export function CHASearch({ value, onChange, options, searchQuery, onSearchQueryChange }: CHASearchProps) {
  const [isFocused, setIsFocused] = useState(false)

  const filtered = useMemo(() => {
    const q = searchQuery.trim().toLowerCase()
    if (!q) return []
    return options.filter((o) => o.name.toLowerCase().includes(q))
  }, [options, searchQuery])

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
  const loading = false

  return (
    <Box position="relative">
      <TextField.Root
        size="2"
        placeholder="Search by CHA (e.g., Spectra, Advantis)..."
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
          className="bg-white border border-gray-200 rounded-md shadow-lg z-10 overflow-hidden"
        >
          {filtered.length === 0 ? (
            <Flex align="center" justify="center" py="5" direction="column" gap="1">
              <Text size="2" color="gray">
                No CHAs found for "{searchQuery}"
              </Text>
            </Flex>
          ) : (
            <ScrollArea style={{ maxHeight: '280px' }}>
              <Box py="1">
                {filtered.map((cha) => {
                  const isSelected = value?.id === cha.id
                  return (
                    <Flex
                      key={cha.id}
                      px="3"
                      py="2"
                      gap="3"
                      align="start"
                      className={`cursor-pointer transition-colors ${
                        isSelected ? 'bg-blue-50 hover:bg-blue-100' : 'hover:bg-gray-50'
                      }`}
                      onMouseDown={(e) => {
                        e.preventDefault()
                        handleSelect(cha)
                      }}
                    >
                      <Box pt="1">
                        <MagnifyingGlassIcon height="14" width="14" className="text-gray-400" />
                      </Box>
                      <Box style={{ flex: 1, minWidth: 0 }}>
                        <Flex align="center" gap="2" mb="1">
                          <Text size="2" weight="medium">
                            {cha.name}
                          </Text>
                          <Badge size="1" color="blue" variant="soft">
                            CHA
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
