import { render, screen } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { describe, expect, it } from 'vitest'
import { Breadcrumb } from '@/components/Breadcrumb'

describe('Breadcrumb', () => {
  it('links each level above the current page and marks the last as current', () => {
    render(
      <MemoryRouter>
        <Breadcrumb items={[{ label: 'Consignments', to: '/consignments' }, { label: 'CN-001' }]} />
      </MemoryRouter>,
    )

    expect(screen.getByRole('link', { name: 'Consignments' }).getAttribute('href')).toBe('/consignments')
    expect(screen.queryByRole('link', { name: 'CN-001' })).toBeNull()
    expect(screen.getByText('CN-001').getAttribute('aria-current')).toBe('page')
  })
})
