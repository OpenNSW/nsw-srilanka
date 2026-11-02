import { render } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { Surface } from './Surface'

describe('Surface', () => {
  it('renders the element asked for, with its elevation and the caller classes', () => {
    const { container } = render(
      <Surface as="details" elevation="sm" className="overflow-hidden">
        <summary>Audit</summary>
      </Surface>,
    )

    const panel = container.firstElementChild!
    expect(panel.tagName).toBe('DETAILS')
    expect(panel.className).toBe('rounded-2xl bg-app-surface shadow-sm overflow-hidden')
  })
})
