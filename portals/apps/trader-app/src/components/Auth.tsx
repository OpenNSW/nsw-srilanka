import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { useAuth } from 'react-oidc-context'
import { DropdownMenu, Avatar, Badge, Flex, Text, Button } from '@radix-ui/themes'
import { ExitIcon } from '@radix-ui/react-icons'
import { useRoles } from '@/hooks/useRoles'

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
  const roles = useRoles()
  const profile = auth.user?.profile
  // The person's name when the token has one; the email is then shown under
  // it, and otherwise once, as the heading.
  const personName =
    (profile?.name as string) ||
    [profile?.given_name, profile?.family_name].filter((part): part is string => Boolean(part)).join(' ')
  const email = (profile?.email as string) || ''
  const name = personName || email || t('auth.userFallback')
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
        <button className="flex items-center gap-2 p-1 rounded-full hover:bg-app-surface-muted transition-colors cursor-pointer focus:outline-none">
          <Avatar size="2" fallback={initials} radius="full" highContrast />
        </button>
      </DropdownMenu.Trigger>
      <DropdownMenu.Content align="end" size="2" style={{ minWidth: 260, maxWidth: 320 }}>
        <Flex direction="column" gap="1" px="3" py="2">
          <Text size="2" weight="bold">
            {name}
          </Text>
          {personName && email && (
            <Text size="1" color="gray">
              {email}
            </Text>
          )}
        </Flex>
        {roles.length > 0 && (
          <>
            <DropdownMenu.Separator />
            <DropdownMenu.Label>{t('auth.userMenu.roles')}</DropdownMenu.Label>
            <Flex direction="column" gap="3" px="3" pt="1" pb="2">
              {roles.map(({ role, title, description }) => (
                <Flex key={role} direction="column" gap="1" align="start">
                  <Badge variant="soft" size="2">
                    {title}
                  </Badge>
                  {description && (
                    <Text size="1" color="gray">
                      {description}
                    </Text>
                  )}
                </Flex>
              ))}
            </Flex>
          </>
        )}
        <DropdownMenu.Separator />
        <DropdownMenu.Item color="red" onClick={onSignOut} style={{ cursor: 'pointer' }}>
          <ExitIcon />
          {t('auth.unauthorized.signOut')}
        </DropdownMenu.Item>
      </DropdownMenu.Content>
    </DropdownMenu.Root>
  )
}
