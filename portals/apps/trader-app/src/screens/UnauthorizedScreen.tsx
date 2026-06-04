import { SignedIn } from '@asgardeo/react'
import { Button } from '@radix-ui/themes'
import { useSignOutHandler } from '../hooks/useSignOutHandler'

export function UnauthorizedScreen() {
  const handleSignOut = useSignOutHandler()

  return (
    <div className="min-h-screen bg-surface">
      <main className="mt-16 min-h-[calc(100vh-64px)] flex items-center justify-center px-6">
        <div className="w-full max-w-lg rounded-xl border border-border bg-background p-8 shadow-sm text-center">
          <h1 className="text-2xl font-semibold text-foreground">Access Restricted</h1>
          <p className="mt-3 text-foreground-muted">
            Your account is signed in, but it does not currently have an application role.
          </p>
          <div className="mt-8 flex items-center justify-center">
            <SignedIn>
              <Button onClick={handleSignOut} size="4" style={{ cursor: 'pointer' }}>
                Sign out
              </Button>
            </SignedIn>
          </div>
        </div>
      </main>
    </div>
  )
}
