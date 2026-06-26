import { Button } from '@radix-ui/themes'
import { useTranslation } from 'react-i18next'
import { useSignOutHandler } from '../hooks/useSignOutHandler'

export function UnauthorizedScreen() {
  const handleSignOut = useSignOutHandler()
  const { t } = useTranslation()

  return (
    <div className="min-h-screen bg-surface relative pb-12">
      <main className="mt-16 min-h-[calc(100vh-64px)] flex items-center justify-center px-6">
        <div className="w-full max-w-lg rounded-xl border border-border bg-background p-8 shadow-sm text-center">
          <h1 className="text-2xl font-semibold text-foreground">{t('auth.unauthorized.title')}</h1>
          <p className="mt-3 text-foreground-muted">{t('auth.unauthorized.message')}</p>
          <div className="mt-8 flex items-center justify-center">
            <Button onClick={handleSignOut} size="4" style={{ cursor: 'pointer' }}>
              {t('auth.unauthorized.signOut')}
            </Button>
          </div>
        </div>
      </main>
      <p className="absolute bottom-4 left-1/2 -translate-x-1/2 text-xs text-foreground-muted">
        {import.meta.env.VITE_APP_VERSION ?? 'dev'}
      </p>
    </div>
  )
}
