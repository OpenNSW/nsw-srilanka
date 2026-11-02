import { render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { defaultUiLabels, UiLabelsProvider } from '../labels'
import { PaginationControl } from './PaginationControl'

const props = { currentPage: 2, totalPages: 3, onPageChange: () => {}, hasNext: true, hasPrev: true, totalCount: 120 }

describe('PaginationControl', () => {
  it('uses English labels when the app sets none', () => {
    render(<PaginationControl {...props} />)

    expect(screen.getByText('Total: 120')).toBeTruthy()
    expect(screen.getByText('Page 2 of 3')).toBeTruthy()
    expect(screen.getByRole('button', { name: 'Next' })).toBeTruthy()
  })

  it("uses the app's labels from UiLabelsProvider", () => {
    render(
      <UiLabelsProvider
        labels={{
          ...defaultUiLabels,
          pagination: {
            total: (count) => `මුළු: ${count}`,
            page: (page, totalPages) => `${page} / ${totalPages}`,
            previous: 'පෙර',
            next: 'ඊළඟ',
          },
        }}
      >
        <PaginationControl {...props} />
      </UiLabelsProvider>,
    )

    expect(screen.getByText('මුළු: 120')).toBeTruthy()
    expect(screen.getByText('2 / 3')).toBeTruthy()
    expect(screen.getByRole('button', { name: 'ඊළඟ' })).toBeTruthy()
  })
})
