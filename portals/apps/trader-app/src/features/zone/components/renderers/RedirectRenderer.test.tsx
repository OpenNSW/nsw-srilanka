import { render, screen } from '@testing-library/react'
import { Theme } from '@radix-ui/themes'
import { afterEach, describe, expect, it } from 'vitest'
import '@/i18n'
import { RedirectRenderer } from '@/features/zone/components/renderers/RedirectRenderer'

// Not an http(s) URL, so the redirect is recorded but jsdom is never asked to navigate.
const payload = { checkout_url: 'test-checkout:session-1', content: 'Pay the fee at the gateway.' }
const redirectedKey = `nsw:redirected:${payload.checkout_url}`

function renderRedirect(interactive: boolean) {
  render(
    <Theme>
      <RedirectRenderer payload={payload} interactive={interactive} />
    </Theme>,
  )
}

describe('RedirectRenderer', () => {
  afterEach(() => {
    sessionStorage.clear()
  })

  it('heads for the checkout in a zone the user may act on', () => {
    renderRedirect(true)

    expect(screen.getByText('Redirecting to payment gateway…')).toBeTruthy()
    expect(sessionStorage.getItem(redirectedKey)).toBe('true')
  })

  it('only shows the content in a zone the user may not act on', () => {
    renderRedirect(false)

    expect(screen.getByText('Pay the fee at the gateway.')).toBeTruthy()
    expect(screen.queryByText('Redirecting to payment gateway…')).toBeNull()
    expect(screen.queryByRole('button')).toBeNull()
    expect(sessionStorage.getItem(redirectedKey)).toBeNull()
  })
})
