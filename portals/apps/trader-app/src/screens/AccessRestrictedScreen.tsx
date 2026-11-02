import { Button } from '@radix-ui/themes'
import { useTranslation } from 'react-i18next'
import { useSignOutHandler } from '@/hooks/useSignOutHandler'

// Shown in place of a screen the signed-in user's scopes don't reach.
export function AccessRestrictedScreen() {
  const handleSignOut = useSignOutHandler()
  const { t } = useTranslation()

  return (
    <div className="flex items-center justify-center px-6 py-16">
      <div className="w-full max-w-lg rounded-2xl bg-app-surface p-8 shadow-md text-center">
        <h1 className="text-2xl font-semibold text-foreground">{t('auth.unauthorized.title')}</h1>
        <p className="mt-3 text-foreground-muted">{t('auth.unauthorized.message')}</p>
        <div className="mt-8 flex items-center justify-center">
          <Button onClick={handleSignOut} size="4" className="cursor-pointer">
            {t('auth.unauthorized.signOut')}
          </Button>
        </div>
      </div>
    </div>
  )
}
