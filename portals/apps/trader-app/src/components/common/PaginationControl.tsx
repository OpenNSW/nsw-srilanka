import { Button, Flex, Text } from '@radix-ui/themes'
import { ChevronLeftIcon, ChevronRightIcon } from '@radix-ui/react-icons'
import { useTranslation } from 'react-i18next'

interface PaginationControlProps {
  currentPage: number
  totalPages: number
  onPageChange: (page: number) => void
  hasNext: boolean
  hasPrev: boolean
  totalCount?: number
}

export function PaginationControl({
  currentPage,
  totalPages,
  onPageChange,
  hasNext,
  hasPrev,
  totalCount,
}: PaginationControlProps) {
  const { t } = useTranslation()

  return (
    <Flex
      justify="between"
      align="center"
      mt="4"
      pt="4"
      pb="4"
      pl="4"
      pr="4"
      style={{ borderTop: '1px solid var(--gray-5)' }}
    >
      <Flex align="center" gap="4">
        {totalCount !== undefined && (
          <Text size="2" color="gray">
            {t('common.pagination.total', { count: totalCount })}
          </Text>
        )}
      </Flex>

      <Flex gap="2" align="center">
        <Text size="2" color="gray" mr="2">
          {t('common.pagination.page', { page: currentPage, totalPages: totalPages || 1 })}
        </Text>
        <Button variant="soft" disabled={!hasPrev} onClick={() => onPageChange(currentPage - 1)}>
          <ChevronLeftIcon />
          {t('common.pagination.previous')}
        </Button>
        <Button variant="soft" disabled={!hasNext} onClick={() => onPageChange(currentPage + 1)}>
          {t('common.pagination.next')}
          <ChevronRightIcon />
        </Button>
      </Flex>
    </Flex>
  )
}
