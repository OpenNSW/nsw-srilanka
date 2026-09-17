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
    <header className="fixed top-3 left-3 right-3 z-50 h-16 rounded-2xl border border-border/60 bg-app-surface/80 shadow-lg backdrop-blur-xl flex items-center justify-between px-6">
      <div className="flex items-center gap-3 min-w-0">
        <BrandMark />
        <span className="text-xl font-bold text-foreground tracking-tight">{displayName}</span>
        <div className="flex items-center pl-4 ml-1 border-l border-border">
          <NavMenu />
        </div>
        {profile?.company?.name && (
          <div className="flex items-center pl-4 ml-1 border-l border-border h-6 min-w-0">
            <span
              className="inline-flex items-center max-w-72 truncate rounded-full bg-primary-subtle px-3 py-1 text-sm font-medium text-primary border border-primary-subtle"
              title={profile.company.name}
            >
              {profile.company.name}
            </span>
          </div>
        )}
      </div>

      <div className="flex items-center gap-4">{children}</div>
    </header>
  )
}

function TopBarUserActions({ onSignOut, withDivider = true }: { onSignOut: () => void; withDivider?: boolean }) {
  return (
    <div className={`flex items-center gap-3 ${withDivider ? 'pl-3 border-l border-border' : ''}`}>
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
