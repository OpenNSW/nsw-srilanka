import { render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { PageHeader } from './PageHeader'

describe('PageHeader', () => {
  it('shows the count beside the title once there is an item', () => {
    render(<PageHeader title="Consignments" count={12} actions={<button>Create</button>} />)

    expect(screen.getByRole('heading', { level: 1, name: 'Consignments' })).toBeTruthy()
    expect(screen.getByText('12')).toBeTruthy()
    expect(screen.getByRole('button', { name: 'Create' })).toBeTruthy()
  })

  it('hides a count of zero', () => {
    render(<PageHeader title="Cases" count={0} />)

    expect(screen.queryByText('0')).toBeNull()
  })
})
