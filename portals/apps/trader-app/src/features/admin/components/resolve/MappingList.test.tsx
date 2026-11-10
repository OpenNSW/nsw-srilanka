import { render, screen } from '@testing-library/react'
import { Theme } from '@radix-ui/themes'
import { describe, expect, it } from 'vitest'
import { MappingList } from '@/features/admin/components/resolve/MappingList'

describe('MappingList', () => {
  it('shows each key without its marker and says what the marker means', () => {
    render(
      <Theme>
        <MappingList
          title="Output mapping"
          note="task result → workflow variable"
          mapping={{
            'containerCount?!': 'userform.containerCount',
            'remarks?': 'review.remarks',
            'flag!': 'review.flag', // "!" alone is not a marker
            outcome: 'review.outcome',
          }}
        />
      </Theme>,
    )

    expect(screen.getByText(/^containerCount/).textContent).toBe(
      'containerCount (optional, removed when absent) → userform.containerCount',
    )
    expect(screen.getByText(/^remarks/).textContent).toBe('remarks (optional) → review.remarks')
    expect(screen.getByText(/^flag!/).textContent).toBe('flag! → review.flag')
    expect(screen.getByText(/^outcome/).textContent).toBe('outcome → review.outcome')
  })
})
