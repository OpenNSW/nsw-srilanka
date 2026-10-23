import { HamburgerMenuIcon } from '@radix-ui/react-icons'
import { Flex, IconButton, Popover } from '@radix-ui/themes'
import { type ReactNode, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { SignedIn, SignedOut, SignInButton, UserDropdown } from '@/components/Auth'
import { useSignOutHandler } from '@/hooks/useSignOutHandler'
import { LanguageSwitcher } from './LanguageSwitcher'
import { NavMenu } from './NavMenu'
import { appConfig, displayName } from '@/config'
import { useProfile } from '@/services/useProfile'

function BrandMark() {
  if (appConfig.branding.systemLogoUrl) {
    return <img src={appConfig.branding.systemLogoUrl} alt={displayName} className="h-8 w-auto object-contain" />
  }
  const initial = displayName.trim().charAt(0).toUpperCase() || 'N'
  return (
    <span className="flex h-9 w-9 shrink-0 items-center justify-center rounded-xl bg-gradient-to-br from-primary to-primary-hover text-sm font-bold text-white shadow-md shadow-primary/30">
      {initial}
    </span>
  )
}

function BuildingIcon() {
  return (
    <svg width="15" height="15" viewBox="0 0 15 15" fill="none" aria-hidden className="shrink-0">
      <path
        d="M3 13.5V2.5a1 1 0 0 1 1-1h5a1 1 0 0 1 1 1v11m-7 0h9m-2 0V6.5a1 1 0 0 0-1-1h-.5M5 4h2M5 6.5h2M5 9h2M6 13.5v-2"
        stroke="currentColor"
        strokeLinecap="round"
        strokeLinejoin="round"
      />
    </svg>
  )
}

// The trader's company, as plain text: it isn't a link, so it doesn't share
// the nav link's pill look.
function CompanyName() {
  const { profile } = useProfile()
  const name = profile?.company?.name
  if (!name) return null
  return (
    <span className="inline-flex min-w-0 max-w-56 items-center gap-1.5 text-sm text-foreground-muted" title={name}>
      <BuildingIcon />
      <span className="truncate">{name}</span>
    </span>
  )
}

// Below 1024 px the bar has room only for the brand mark, this menu and the
// avatar, so everything else moves in here.
function SmallScreenMenu() {
  const { t } = useTranslation()
  const [open, setOpen] = useState(false)
  return (
    <Popover.Root open={open} onOpenChange={setOpen}>
      <Popover.Trigger>
        <IconButton type="button" variant="ghost" color="gray" size="3" aria-label={t('nav.menu')}>
          <HamburgerMenuIcon width="20" height="20" />
        </IconButton>
      </Popover.Trigger>
      <Popover.Content align="end" width="320px" maxWidth="calc(100vw - 32px)">
        <Flex direction="column" gap="3" align="start">
          <NavMenu onNavigate={() => setOpen(false)} />
          <CompanyName />
          <LanguageSwitcher />
        </Flex>
      </Popover.Content>
    </Popover.Root>
  )
}

// Below 1024 px the bar spans the top edge, solid; from 1024 px up it floats
// 12 px in. Layout.tsx clears it with --app-top-offset (index.css).
function TopBarShell({ children }: { children: ReactNode }) {
  return (
    <header className="fixed inset-x-0 top-0 z-50 flex h-16 items-center justify-between gap-3 bg-app-surface px-4 shadow-sm lg:top-3 lg:left-3 lg:right-3 lg:rounded-2xl lg:bg-app-surface/80 lg:px-6 lg:shadow-lg lg:backdrop-blur-xl">
      <div className="flex min-w-0 items-center gap-6">
        <div className="flex items-center gap-3">
          <BrandMark />
          <span className="sr-only text-xl font-bold tracking-tight text-foreground lg:not-sr-only">{displayName}</span>
        </div>
        <div className="hidden lg:flex">
          <NavMenu />
        </div>
      </div>

      <div className="flex items-center gap-3 lg:gap-5">{children}</div>
    </header>
  )
}

function TopBarUserActions({ onSignOut }: { onSignOut: () => void }) {
  return (
    <div className="flex items-center gap-3">
      <SignedIn>
        <UserDropdown onSignOut={onSignOut} />
      </SignedIn>
      <SignedOut>
        <SignInButton />
      </SignedOut>
    </div>
  )
}

export function TopBar() {
  const handleSignOut = useSignOutHandler()

  return (
    <TopBarShell>
      <div className="hidden items-center gap-5 lg:flex">
        <LanguageSwitcher />
        <CompanyName />
      </div>
      <div className="lg:hidden">
        <SmallScreenMenu />
      </div>
      <TopBarUserActions onSignOut={handleSignOut} />
    </TopBarShell>
  )
}
