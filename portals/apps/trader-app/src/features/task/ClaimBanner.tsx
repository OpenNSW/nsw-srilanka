import { AlertDialog, Badge, Button, Flex, Spinner, Text } from '@radix-ui/themes'
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
  // Claims the task, taking it over when another user holds it.
  onClaim: () => void
  onRelease: () => void
}

// ClaimBanner says who holds the task's claim and offers Claim, Release or Take over.
// A claim is a hint for other users: it never locks the task.
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

  return (
    <div className="rounded-2xl bg-app-surface shadow-sm px-4 py-3">
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
          <AlertDialog.Root>
            <AlertDialog.Trigger>
              <Button variant="soft" size="2" disabled={busy} className="cursor-pointer">
                {busy ? <Spinner size="1" /> : <LockClosedIcon />}
                {t('tasks.claim.takeOver')}
              </Button>
            </AlertDialog.Trigger>
            <AlertDialog.Content maxWidth="450px">
              <AlertDialog.Title>{t('tasks.claim.takeOverTitle')}</AlertDialog.Title>
              <AlertDialog.Description size="2">
                {claim.holder.name
                  ? t('tasks.claim.takeOverConfirm', { name: claim.holder.name })
                  : t('tasks.claim.takeOverConfirmUnnamed')}
              </AlertDialog.Description>
              <Flex gap="3" mt="4" justify="end">
                <AlertDialog.Cancel>
                  <Button variant="soft" color="gray">
                    {t('tasks.claim.cancel')}
                  </Button>
                </AlertDialog.Cancel>
                <AlertDialog.Action>
                  <Button onClick={onClaim}>{t('tasks.claim.takeOver')}</Button>
                </AlertDialog.Action>
              </Flex>
            </AlertDialog.Content>
          </AlertDialog.Root>
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
