import { render, screen } from '@testing-library/react'
import { Theme } from '@radix-ui/themes'
import { describe, expect, it } from 'vitest'
import '@/i18n'
import { ClaimBanner } from '@/features/task/ClaimBanner'

function renderBanner(canChange: boolean) {
  render(
    <Theme>
      <ClaimBanner
        claim={{ mine: false }}
        completed={false}
        canChange={canChange}
        busy={false}
        error={null}
        onClaim={() => undefined}
        onRelease={() => undefined}
      />
    </Theme>,
  )
}

describe('ClaimBanner', () => {
  it('offers Claim to a user who may change claims', () => {
    renderBanner(true)

    expect(screen.getByRole('button', { name: 'Claim' })).toBeTruthy()
  })

  it('only says who holds the claim to a user who may not', () => {
    renderBanner(false)

    expect(screen.getByText('Unclaimed')).toBeTruthy()
    expect(screen.queryByRole('button')).toBeNull()
  })
})
