import { useState, type FormEvent } from 'react'
import { useNavigate } from 'react-router-dom'
import { Button, TextField } from '@radix-ui/themes'
import { useTranslation } from 'react-i18next'
import { ROUTES } from '@/constants'
import { isAgencyMode } from '@/features/case/mode'
import { useSignOutHandler } from '@/hooks/useSignOutHandler'
import { useAbilities } from '@/hooks/useCan'

// Shown where the signed-in user has no screen to open. An admin is told their
// account is fine and can open a consignment (a case, in an agency portal) by
// ID; any other user is told it lacks access.
export function AccessRestrictedScreen() {
  const handleSignOut = useSignOutHandler()
  const navigate = useNavigate()
  const { t } = useTranslation()
  const { can } = useAbilities()
  const [id, setId] = useState('')
  const isAdmin = can('admin.view') || can('admin.intervene')

  const openAdminView = (event: FormEvent) => {
    event.preventDefault()
    const trimmed = id.trim()
    if (trimmed) void navigate(ROUTES.ADMIN_CONSIGNMENT(encodeURIComponent(trimmed)))
  }

  return (
    <div className="flex items-center justify-center px-6 py-16">
      <div className="w-full max-w-lg rounded-2xl bg-app-surface p-8 shadow-md text-center">
        <h1 className="text-2xl font-semibold text-foreground">
          {isAdmin ? t('auth.unauthorized.adminTitle') : t('auth.unauthorized.title')}
        </h1>
        <p className="mt-3 text-foreground-muted">
          {isAdmin ? t('auth.unauthorized.adminMessage') : t('auth.unauthorized.message')}
        </p>
        {can('admin.view') && (
          <>
            <p className="mt-6 text-sm text-foreground-muted">
              {isAgencyMode
                ? t('auth.unauthorized.adminCaseIdPrompt')
                : t('auth.unauthorized.adminConsignmentIdPrompt')}
            </p>
            <form onSubmit={openAdminView} className="mt-2 flex items-center gap-2">
              <TextField.Root
                value={id}
                onChange={(event) => setId(event.target.value)}
                placeholder={
                  isAgencyMode
                    ? t('auth.unauthorized.adminCaseIdPlaceholder')
                    : t('auth.unauthorized.adminConsignmentIdPlaceholder')
                }
                size="3"
                required
                className="flex-1"
              />
              <Button type="submit" size="3" className="cursor-pointer">
                {t('auth.unauthorized.adminOpen')}
              </Button>
            </form>
          </>
        )}
        <div className="mt-8 flex items-center justify-center">
          <Button onClick={handleSignOut} size="4" className="cursor-pointer">
            {t('auth.unauthorized.signOut')}
          </Button>
        </div>
      </div>
    </div>
  )
}
