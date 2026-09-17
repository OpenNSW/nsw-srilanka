import { BellIcon } from '@radix-ui/react-icons'
import { type ReactNode } from 'react'
import { SignedIn, SignedOut, SignInButton, UserDropdown } from '@/components/Auth'
import { useSignOutHandler } from '@/hooks/useSignOutHandler'
import { RoleSwitcher } from './RoleSwitcher'
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

function TopBarShell({ children }: { children: ReactNode }) {
  const { profile } = useProfile()

  return (
    <header className="fixed top-3 left-3 right-3 z-50 h-16 rounded-2xl bg-app-surface/80 shadow-lg backdrop-blur-xl flex items-center justify-between px-6">
      <div className="flex items-center gap-6 min-w-0">
        <div className="flex items-center gap-3">
          <BrandMark />
          <span className="text-xl font-bold text-foreground tracking-tight">{displayName}</span>
        </div>
        <NavMenu />
        {profile?.company?.name && (
          <span
            className="inline-flex items-center max-w-72 truncate rounded-full bg-primary-subtle px-3 py-1 text-sm font-medium text-primary"
            title={profile.company.name}
          >
            {profile.company.name}
          </span>
        )}
      </div>

      <div className="flex items-center gap-5">{children}</div>
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
      <RoleSwitcher />
      {/* Notifications */}
      {/* TODO: Show real notifications and link to a notifications page */}
      <button className="relative p-2 text-foreground-subtle hover:text-foreground-muted hover:bg-app-surface-muted rounded-lg transition-colors">
        <BellIcon className="w-5 h-5" />
        <span className="absolute top-1.5 right-1.5 w-2 h-2 bg-error rounded-full"></span>
      </button>
      <LanguageSwitcher />
      <TopBarUserActions onSignOut={handleSignOut} />
    </TopBarShell>
  )
}
