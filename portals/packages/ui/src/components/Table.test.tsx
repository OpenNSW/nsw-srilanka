import { fireEvent, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import { describe, expect, it } from 'vitest'
import { LinkRow, RowLink } from './LinkRow'
import { Table } from './Table'

function List() {
  return (
    <Table>
      <Table.Header>
        <Table.HeaderCell>ID</Table.HeaderCell>
        <Table.HeaderCell>State</Table.HeaderCell>
      </Table.Header>
      <Table.Body>
        <LinkRow to="/consignments/CN-1">
          <Table.Cell>
            <RowLink to="/consignments/CN-1">CN-1</RowLink>
          </Table.Cell>
          <Table.Cell>In progress</Table.Cell>
        </LinkRow>
      </Table.Body>
    </Table>
  )
}

function renderAt(path: string) {
  render(
    <MemoryRouter initialEntries={[path]}>
      <Routes>
        <Route path="/consignments" element={<List />} />
        <Route path="/consignments/:id" element={<p>Opened</p>} />
      </Routes>
    </MemoryRouter>,
  )
}

describe('Table', () => {
  it('is a table with its column headers', () => {
    renderAt('/consignments')

    expect(screen.getAllByRole('columnheader').map((cell) => cell.textContent)).toEqual(['ID', 'State'])
    expect(screen.getByRole('cell', { name: 'In progress' })).toBeTruthy()
  })

  it("opens a LinkRow's page from a click anywhere on the row", () => {
    renderAt('/consignments')

    fireEvent.click(screen.getByRole('cell', { name: 'In progress' }))
    expect(screen.getByText('Opened')).toBeTruthy()
  })

  it("opens a LinkRow's page from the keyboard: Tab to its RowLink, then Enter", async () => {
    const user = userEvent.setup()
    renderAt('/consignments')

    await user.tab()
    expect(document.activeElement).toBe(screen.getByRole('link', { name: 'CN-1' }))
    await user.keyboard('{Enter}')
    expect(screen.getByText('Opened')).toBeTruthy()
  })
})
