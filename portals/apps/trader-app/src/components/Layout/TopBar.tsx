import { BellIcon } from '@radix-ui/react-icons'
import { SignedIn, SignedOut, SignInButton, UserDropdown } from '@asgardeo/react'
import { type ReactNode } from 'react'
import { useSignOutHandler } from '../../hooks/useSignOutHandler'
import { RoleSwitcher } from './RoleSwitcher'
import { appConfig, displayName } from '../../config'

function TopBarShell({ children }: { children: ReactNode }) {
  return (
    <header className="fixed top-0 left-0 right-0 z-50 h-16 bg-background border-b border-border flex items-center justify-between px-6">
      <div className="flex items-center gap-3">
        {appConfig.branding.systemLogoUrl && (
          <img src={appConfig.branding.systemLogoUrl} alt={displayName} className="h-8 w-auto object-contain" />
        )}
        <span className="text-xl font-bold text-foreground">{displayName}</span>
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
      <button className="relative p-2 text-foreground-subtle hover:text-foreground-muted hover:bg-surface-muted rounded-lg transition-colors">
        <BellIcon className="w-5 h-5" />
        <span className="absolute top-1.5 right-1.5 w-2 h-2 bg-error rounded-full"></span>
      </button>
      <TopBarUserActions onSignOut={handleSignOut} />
    </TopBarShell>
  )
}
