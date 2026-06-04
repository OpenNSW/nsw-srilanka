import { useState, useEffect, useRef } from 'react'
import { Text, Box, Flex, Badge, Spinner, TextField, ScrollArea, IconButton } from '@radix-ui/themes'
import { MagnifyingGlassIcon, Cross2Icon } from '@radix-ui/react-icons'
import { getHSCodes } from '../../services/hsCode.ts'
import { useApi } from '../../services/ApiContext'
import type { HSCode } from '../../services/types/hsCode.ts'

interface HSCodeSearchProps {
  value: HSCode | null
  onChange: (hsCode: HSCode | null) => void
}

function getCategoryColor(category: string): 'blue' | 'green' | 'orange' | 'purple' {
  const lowerCategory = category.toLowerCase()
  if (lowerCategory.includes('green tea')) return 'green'
  if (lowerCategory.includes('black tea')) return 'orange'
  if (lowerCategory.includes('instant') || lowerCategory.includes('value')) return 'purple'
  return 'blue'
}

export function HSCodeSearch({ value, onChange }: HSCodeSearchProps) {
  const api = useApi()
  const [searchQuery, setSearchQuery] = useState('')
  const [hsCodes, setHsCodes] = useState<HSCode[]>([])
  const [loading, setLoading] = useState(false)
  const [isFocused, setIsFocused] = useState(false)
  const searchRequestIdRef = useRef(0)

  useEffect(() => {
    async function fetchHSCodes() {
      if (!searchQuery) {
        searchRequestIdRef.current += 1
        setHsCodes([])
        setLoading(false)
        return
      }

      const requestId = ++searchRequestIdRef.current
      setLoading(true)
      try {
        const result = await getHSCodes(
          {
            hsCodeStartsWith: searchQuery,
            limit: 20,
          },
          api,
        )
        if (requestId !== searchRequestIdRef.current) {
          return
        }
        setHsCodes(result.items)
      } catch (error) {
        if (requestId !== searchRequestIdRef.current) {
          return
        }
        console.error('Failed to fetch HS codes:', error)
      } finally {
        if (requestId === searchRequestIdRef.current) {
          setLoading(false)
        }
      }
    }

    const debounce = setTimeout(fetchHSCodes, 300)
    return () => clearTimeout(debounce)
  }, [api, searchQuery])

  const handleSelect = (hsCode: HSCode) => {
    setSearchQuery(`${hsCode.hsCode} - ${hsCode.description}`)
    onChange(hsCode)
    setIsFocused(false)
  }

  const handleClear = () => {
    setSearchQuery('')
    onChange(null)
  }

  const showDropdown = isFocused && searchQuery.length > 0

  return (
    <Box position="relative">
      <TextField.Root
        size="2"
        placeholder="Search by HS Code (e.g., 01, 8471, 851712)..."
        value={searchQuery}
        onChange={(e) => setSearchQuery(e.target.value)}
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
          {loading && hsCodes.length === 0 ? (
            <Flex align="center" justify="center" py="4">
              <Spinner size="2" />
              <Text size="2" color="gray" ml="2">
                Searching...
              </Text>
            </Flex>
          ) : hsCodes.length === 0 ? (
            <Flex align="center" justify="center" py="5" direction="column" gap="1">
              <Text size="2" color="gray">
                No HS codes found for "{searchQuery}"
              </Text>
            </Flex>
          ) : (
            <ScrollArea style={{ maxHeight: '280px' }}>
              <Box py="1">
                {hsCodes.map((hsCode) => {
                  const isSelected = value?.id === hsCode.id

                  return (
                    <Flex
                      key={hsCode.id}
                      px="3"
                      py="2"
                      gap="3"
                      align="start"
                      className={`cursor-pointer transition-colors ${
                        isSelected ? 'bg-info-subtle hover:bg-info/15' : 'hover:bg-surface'
                      }`}
                      onMouseDown={(e) => {
                        e.preventDefault()
                        handleSelect(hsCode)
                      }}
                    >
                      <Box pt="1">
                        <MagnifyingGlassIcon height="14" width="14" className="text-foreground-subtle" />
                      </Box>
                      <Box style={{ flex: 1, minWidth: 0 }}>
                        <Flex align="center" gap="2" mb="1">
                          <Text size="2" weight="medium">
                            {hsCode.hsCode}
                          </Text>
                          <Badge size="1" color={getCategoryColor(hsCode.category)} variant="soft">
                            {hsCode.category}
                          </Badge>
                        </Flex>
                        <Text size="1" color="gray" className="line-clamp-2">
                          {hsCode.description}
                        </Text>
                      </Box>
                    </Flex>
                  )
                })}
              </Box>
            </ScrollArea>
          )}
        </Box>
      )}

      {!showDropdown && !searchQuery && (
        <Text size="2" color="gray" align="center" as="p" mt="3">
          Start typing to search for HS codes
        </Text>
      )}
    </Box>
  )
}
