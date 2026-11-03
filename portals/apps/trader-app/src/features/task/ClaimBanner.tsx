import { useState } from 'react'
import { Button, Spinner, Text } from '@radix-ui/themes'
import { LockClosedIcon, LockOpen1Icon } from '@radix-ui/react-icons'
import { useTranslation } from 'react-i18next'
import { ConfirmDialog, StatusBadge, Surface } from '@portals/ui'
import type { TaskClaim } from '@/features/zone/types'

type Props = {
  claim: TaskClaim
  // A completed task keeps its claim as a record of who worked on it, but can no
  // longer be claimed, and is not worth releasing.
  completed: boolean
  busy: boolean
  error: string | null
  // Claims the task, taking it over when another user holds it.
  onClaim: () => void
  onRelease: () => void
}

// ClaimBanner says who holds the task's claim and offers Claim, Release or Take over.
// A claim is a hint for other users: it never locks the task.
export function ClaimBanner({ claim, completed, busy, error, onClaim, onRelease }: Props) {
  const { t } = useTranslation()
  const [confirmingTakeOver, setConfirmingTakeOver] = useState(false)
  const name = claim.holder?.name || t('tasks.claim.anotherUser')

  let badge
  if (claim.mine) {
    badge = (
      <StatusBadge tone="success" size="2">
        {t('tasks.claim.claimedByYou')}
      </StatusBadge>
    )
  } else if (claim.holder) {
    badge = (
      <StatusBadge tone="warning" size="2">
        {t('tasks.claim.claimedBy', { name })}
      </StatusBadge>
    )
  } else {
    badge = (
      <StatusBadge tone="neutral" size="2">
        {t('tasks.claim.unclaimed')}
      </StatusBadge>
    )
  }

  return (
    <Surface elevation="sm" className="px-4 py-3">
      <div className="flex flex-wrap items-center justify-between gap-3">
        {badge}
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
        {!completed && claim.holder && !claim.mine && (
          <>
            <Button
              variant="soft"
              size="2"
              onClick={() => setConfirmingTakeOver(true)}
              disabled={busy}
              className="cursor-pointer"
            >
              {busy ? <Spinner size="1" /> : <LockClosedIcon />}
              {t('tasks.claim.takeOver')}
            </Button>
            <ConfirmDialog
              open={confirmingTakeOver}
              onOpenChange={setConfirmingTakeOver}
              title={t('tasks.claim.takeOverTitle')}
              description={
                claim.holder.name
                  ? t('tasks.claim.takeOverConfirm', { name: claim.holder.name })
                  : t('tasks.claim.takeOverConfirmUnnamed')
              }
              confirmLabel={t('tasks.claim.takeOver')}
              cancelLabel={t('tasks.claim.cancel')}
              onConfirm={() => {
                setConfirmingTakeOver(false)
                onClaim()
              }}
            />
          </>
        )}
      </div>
      {error && (
        <Text as="p" size="2" color="red" className="mt-2">
          {error}
        </Text>
      )}
    </Surface>
  )
}
