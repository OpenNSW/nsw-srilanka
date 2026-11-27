import { Theme } from '@radix-ui/themes'
import { fireEvent, render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import {
  ConsignmentSearchCard,
  type SearchCopy,
  type SearchStatus,
} from '@/features/admin/components/search/ConsignmentSearchCard'

const COPY: SearchCopy = {
  title: 'Thing search',
  subtitle: 'Open a thing.',
  idLabel: 'Thing ID',
  helper: 'Find the ID somewhere.',
  notFound: 'Thing not found.',
  error: "Couldn't look up the thing.",
}

interface Props {
  onSearch?: (id: string) => void
  pending?: boolean
  status?: SearchStatus
  onRetry?: () => void
  onChange?: () => void
}

function renderCard(props: Props = {}) {
  const onSearch = props.onSearch ?? vi.fn()
  const result = render(
    <Theme>
      <ConsignmentSearchCard copy={COPY} {...props} onSearch={onSearch} />
    </Theme>,
  )
  const rerender = (next: Props) =>
    result.rerender(
      <Theme>
        <ConsignmentSearchCard copy={COPY} {...next} onSearch={onSearch} />
      </Theme>,
    )
  return { onSearch, rerender }
}

function input() {
  return screen.getByLabelText<HTMLInputElement>('Thing ID')
}

function button() {
  return screen.getByRole<HTMLButtonElement>('button', { name: /Search/ })
}

describe('ConsignmentSearchCard', () => {
  it('shows the copy it is given', () => {
    renderCard()
    expect(input().placeholder).toBe('e.g. 0b6f3c2e-…')
    expect(screen.getByText('Find the ID somewhere.')).toBeTruthy()
    expect(input().getAttribute('aria-describedby')).toBe(screen.getByText('Find the ID somewhere.').id)
  })

  it('searches the trimmed ID on click and on Enter', () => {
    const { onSearch } = renderCard()
    fireEvent.change(input(), { target: { value: '  abc  ' } })
    fireEvent.click(button())
    expect(onSearch).toHaveBeenLastCalledWith('abc')

    fireEvent.submit(input().closest('form')!)
    expect(onSearch).toHaveBeenCalledTimes(2)
  })

  it('keeps Search disabled and searches nothing while the ID is blank', () => {
    const { onSearch } = renderCard()
    fireEvent.change(input(), { target: { value: '   ' } })
    expect(button().disabled).toBe(true)
    fireEvent.submit(input().closest('form')!)
    expect(onSearch).not.toHaveBeenCalled()
  })

  it('locks the field and button while a search is pending', () => {
    const { onSearch, rerender } = renderCard()
    fireEvent.change(input(), { target: { value: 'abc' } })
    rerender({ pending: true })

    expect(input().readOnly).toBe(true)
    expect(button().disabled).toBe(true)
    expect(button().getAttribute('aria-busy')).toBe('true')
    expect(button().textContent).toContain('Searching…')
    fireEvent.submit(input().closest('form')!)
    expect(onSearch).not.toHaveBeenCalled()
  })

  it('shows a not-found message and marks the field invalid', () => {
    renderCard({ status: 'notFound' })
    expect(screen.getByRole('alert').textContent).toContain('Thing not found.')
    expect(input().getAttribute('aria-invalid')).toBe('true')
  })

  it('reports edits, so the screen can clear a message', () => {
    const onChange = vi.fn()
    renderCard({ status: 'notFound', onChange })
    fireEvent.change(input(), { target: { value: 'abd' } })
    expect(onChange).toHaveBeenCalledTimes(1)
  })

  it('shows the error with a Retry button', () => {
    const onRetry = vi.fn()
    renderCard({ status: 'error', onRetry })
    expect(screen.getByRole('alert').textContent).toContain("Couldn't look up the thing.")
    fireEvent.click(screen.getByRole('button', { name: 'Retry' }))
    expect(onRetry).toHaveBeenCalledTimes(1)
  })

  it('shows no message while idle', () => {
    renderCard()
    expect(screen.queryByRole('alert')).toBeNull()
    expect(screen.queryByRole('button', { name: 'Retry' })).toBeNull()
  })
})
