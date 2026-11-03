import { render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { StatusBadge } from './StatusBadge'

describe('StatusBadge', () => {
  it.each([
    ['neutral', 'gray'],
    ['info', 'blue'],
    ['success', 'green'],
    ['warning', 'amber'],
    ['danger', 'red'],
  ] as const)('paints the %s tone %s', (tone, color) => {
    render(<StatusBadge tone={tone}>{tone}</StatusBadge>)

    expect(screen.getByText(tone).getAttribute('data-accent-color')).toBe(color)
  })
})
