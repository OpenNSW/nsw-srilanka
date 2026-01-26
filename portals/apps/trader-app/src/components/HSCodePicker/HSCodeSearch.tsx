import { useState, useEffect } from 'react'
import { Text, Box, Flex, Badge, Spinner, TextField, ScrollArea, IconButton } from '@radix-ui/themes'
import { MagnifyingGlassIcon, Cross2Icon } from '@radix-ui/react-icons'
import {getHSCodes} from "../../services/hsCode.ts";
import type {HSCode} from "../../services/types/hsCode.ts";

interface HSCodeSearchProps {
  value: HSCode | null
  onChange: (hsCode: HSCode | null) => void
}

function getLevelLabel(level: number): string {
  switch (level) {
    case 1:
      return 'Chapter'
    case 2:
      return 'Heading'
    case 3:
      return 'Subheading'
    default:
      return 'Item'
  }
}

function getLevelColor(level: number): 'blue' | 'green' | 'orange' {
  switch (level) {
    case 1:
      return 'blue'
    case 2:
      return 'green'
    case 3:
      return 'orange'
    default:
      return 'blue'
  }
}

export function HSCodeSearch({ value, onChange }: HSCodeSearchProps) {
  const [searchQuery, setSearchQuery] = useState('')
  const [hsCodes, setHsCodes] = useState<HSCode[]>([])
  const [loading, setLoading] = useState(false)
  const [isFocused, setIsFocused] = useState(false)

  useEffect(() => {
    async function fetchHSCodes() {
      if (!searchQuery) {
        setHsCodes([])
        return
      }
      setLoading(true)
      try {
        const result = await getHSCodes({
          hs_code: searchQuery,
          limit: 10,
        })
        setHsCodes(result.data)
      } catch (error) {
        console.error('Failed to fetch HS codes:', error)
      } finally {
        setLoading(false)
      }
    }

    const debounce = setTimeout(fetchHSCodes, 300)
    return () => clearTimeout(debounce)
  }, [searchQuery])

  const handleSelect = (hsCode: HSCode) => {
    setSearchQuery(`${hsCode.code} - ${hsCode.description}`)
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
                        isSelected
                          ? 'bg-blue-50 hover:bg-blue-100'
                          : 'hover:bg-gray-50'
                      }`}
                      onClick={() => handleSelect(hsCode)}
                    >
                      <Box pt="1">
                        <MagnifyingGlassIcon
                          height="14"
                          width="14"
                          className="text-gray-400"
                        />
                      </Box>
                      <Box style={{ flex: 1, minWidth: 0 }}>
                        <Flex align="center" gap="2" mb="1">
                          <Text size="2" weight="medium">
                            {hsCode.code}
                          </Text>
                          <Badge
                            size="1"
                            color={getLevelColor(hsCode.level)}
                            variant="soft"
                          >
                            {getLevelLabel(hsCode.level)}
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