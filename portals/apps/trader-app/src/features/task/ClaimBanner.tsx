import { Badge, Button, Spinner, Text } from '@radix-ui/themes'
import { LockClosedIcon, LockOpen1Icon } from '@radix-ui/react-icons'
import { useTranslation } from 'react-i18next'
import type { TaskClaim } from '@/features/zone/types'

type Props = {
  claim: TaskClaim
  // A completed task keeps its claim as a record of who worked on it, but can no
  // longer be claimed, and is not worth releasing.
  completed: boolean
  busy: boolean
  error: string | null
  onClaim: () => void
  onRelease: () => void
}

// ClaimBanner says who holds the task's claim and offers Claim or Release.
export function ClaimBanner({ claim, completed, busy, error, onClaim, onRelease }: Props) {
  const { t } = useTranslation()
  const name = claim.holder?.name || t('tasks.claim.anotherUser')

  let badge
  if (claim.mine) {
    badge = (
      <Badge size="2" color="green" variant="soft">
        {t('tasks.claim.claimedByYou')}
      </Badge>
    )
  } else if (claim.holder) {
    badge = (
      <Badge size="2" color="amber" variant="soft">
        {t('tasks.claim.claimedBy', { name })}
      </Badge>
    )
  } else {
    badge = (
      <Badge size="2" color="gray" variant="soft">
        {t('tasks.claim.unclaimed')}
      </Badge>
    )
  }

  // Only worth saying where the claim decides who may act.
  let hint: string | null = null
  if (claim.required_to_submit && !completed && !claim.mine) {
    hint = claim.holder ? t('tasks.claim.claimedByOther', { name }) : t('tasks.claim.claimToAct')
  }

  return (
    <div className="rounded-2xl bg-app-surface shadow-sm px-4 py-3">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div className="flex flex-wrap items-center gap-3">
          {badge}
          {hint && (
            <Text size="2" color="gray">
              {hint}
            </Text>
          )}
        </div>
        {!completed && !claim.holder && (
          <Button variant="soft" size="2" onClick={onClaim} disabled={busy} className="cursor-pointer">
            {busy ? <Spinner size="1" /> : <LockClosedIcon />}
            {t('tasks.claim.claim')}
          </Button>
        )}
        {!completed && claim.mine && (
          <Button variant="soft" color="gray" size="2" onClick={onRelease} disabled={busy} className="cursor-pointer">
            {busy ? <Spinner size="1" /> : <LockOpen1Icon />}
            {t('tasks.claim.release')}
          </Button>
        )}
      </div>
      {error && (
        <Text as="p" size="2" color="red" className="mt-2">
          {error}
        </Text>
      )}
    </div>
  )
}
