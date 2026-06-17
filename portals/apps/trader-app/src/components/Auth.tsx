import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { useAuth } from 'react-oidc-context'
import { DropdownMenu, Avatar, Flex, Text, Button } from '@radix-ui/themes'

export function SignedIn({ children }: { children: ReactNode }) {
  const auth = useAuth()
  return auth.isAuthenticated ? <>{children}</> : null
}

export function SignedOut({ children, fallback }: { children: ReactNode; fallback?: ReactNode }) {
  const auth = useAuth()
  if (auth.isLoading) return null
  if (!auth.isAuthenticated) return <>{children}</>
  return fallback ? <>{fallback}</> : null
}

export function SignInButton() {
  const { t } = useTranslation()
  const auth = useAuth()
  return (
    <Button onClick={() => void auth.signinRedirect()} size="2" style={{ cursor: 'pointer' }}>
      {t('auth.login.button')}
    </Button>
  )
}

export function UserDropdown({ onSignOut }: { onSignOut: () => void }) {
  const { t } = useTranslation()
  const auth = useAuth()
  const name =
    (auth.user?.profile?.name as string) || (auth.user?.profile?.email as string) || t('auth.unauthorized.title')
  const email = (auth.user?.profile?.email as string) || ''
  const initials = name
    .split(/\s+/)
    .filter(Boolean)
    .map((n) => n[0])
    .join('')
    .toUpperCase()
    .slice(0, 2)

  return (
    <DropdownMenu.Root>
      <DropdownMenu.Trigger>
        <button className="flex items-center gap-2 p-1 rounded-full hover:bg-surface-muted transition-colors cursor-pointer focus:outline-none">
          <Avatar size="2" fallback={initials} radius="full" color="indigo" />
        </button>
      </DropdownMenu.Trigger>
      <DropdownMenu.Content align="end" size="2" style={{ minWidth: 200 }}>
        <Flex direction="column" gap="1" px="3" py="2">
          <Text size="2" weight="bold">
            {name}
          </Text>
          {email && (
            <Text size="1" color="gray">
              {email}
            </Text>
          )}
        </Flex>
        <DropdownMenu.Separator />
        <DropdownMenu.Item color="red" onClick={onSignOut} style={{ cursor: 'pointer' }}>
          {t('auth.unauthorized.signOut')}
        </DropdownMenu.Item>
      </DropdownMenu.Content>
    </DropdownMenu.Root>
  )
}
