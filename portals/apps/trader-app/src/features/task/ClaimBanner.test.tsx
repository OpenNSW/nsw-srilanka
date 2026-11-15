import { fireEvent, render, screen, within } from '@testing-library/react'
import { Theme } from '@radix-ui/themes'
import { describe, expect, it, vi } from 'vitest'
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

describe('ClaimBanner take over', () => {
  function renderHeldByAnother(onClaim: () => void) {
    render(
      <Theme>
        <ClaimBanner
          claim={{ mine: false, holder: { name: 'Nimal' } }}
          completed={false}
          canChange
          busy={false}
          error={null}
          onClaim={onClaim}
          onRelease={() => undefined}
        />
      </Theme>,
    )
  }

  it('asks before taking over, then closes and claims', () => {
    const onClaim = vi.fn()
    renderHeldByAnother(onClaim)

    fireEvent.click(screen.getByRole('button', { name: 'Take over' }))
    const dialog = screen.getByRole('alertdialog', { name: 'Take over this task?' })
    expect(within(dialog).getByText(/Nimal is working on this task/)).toBeTruthy()

    fireEvent.click(within(dialog).getByRole('button', { name: 'Take over' }))

    expect(onClaim).toHaveBeenCalledTimes(1)
    expect(screen.queryByRole('alertdialog')).toBeNull()
  })

  it('takes nothing over when cancelled', () => {
    const onClaim = vi.fn()
    renderHeldByAnother(onClaim)

    fireEvent.click(screen.getByRole('button', { name: 'Take over' }))
    fireEvent.click(screen.getByRole('button', { name: 'Cancel' }))

    expect(onClaim).not.toHaveBeenCalled()
    expect(screen.queryByRole('alertdialog')).toBeNull()
  })
})
