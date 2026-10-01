import { useEffect, useMemo } from 'react'
import { useAuth } from 'react-oidc-context'
import { useTranslation } from 'react-i18next'
import { appConfig, displayName } from '@/config'
import { LanguageSwitcher } from '@/components/Layout/LanguageSwitcher'
import { supportedLanguages } from '@/i18n'

export function LoginScreen() {
  const auth = useAuth()
  const { t } = useTranslation()

  const hasUrlError = useMemo(() => new URLSearchParams(window.location.search).has('error'), [])

  useEffect(() => {
    if (!hasUrlError) {
      void auth.clearStaleState()
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  const { systemName, appName, logoUrl, description, heroImageUrl, partnerLogos } = appConfig.branding

  return (
    <div className="min-h-screen flex flex-col lg:flex-row bg-app-surface lg:overflow-hidden overflow-y-auto">
      {/* Frosted so it's legible over both the white identity panel and the dark hero image behind it.
          Only rendered when there's an actual choice — LanguageSwitcher itself returns null otherwise,
          which would leave this an empty floating box. */}
      {supportedLanguages.length > 1 && (
        <div className="fixed top-4 right-4 z-40 rounded-lg bg-white/90 shadow-sm backdrop-blur-sm">
          <LanguageSwitcher />
        </div>
      )}

      {/* Mobile logo strip — full-width box at the very top, hidden on desktop */}
      {logoUrl && (
        <div className="lg:hidden w-full bg-app-surface px-6 py-4 flex items-center justify-center shadow-sm">
          <img src={logoUrl} alt={appName} className="h-16 object-contain" />
        </div>
      )}

      {/* Hero & Authentication */}
      <div className="lg:order-last relative flex-1 min-h-125 lg:min-h-[calc(100vh-2rem)] overflow-hidden">
        {/* Hero background — no clip on mobile (logo has its own strip above), diagonal on desktop */}
        <div className="absolute inset-0 [clip-path:none] lg:[clip-path:polygon(25%_0,100%_0,100%_100%,0%_100%)]">
          <div
            className={`absolute inset-0 bg-cover bg-center ${heroImageUrl ? '' : 'bg-gradient-to-br from-primary-dark via-primary to-primary-dark'}`}
            style={heroImageUrl ? { backgroundImage: `url(${heroImageUrl})` } : undefined}
          />
          <div className="absolute inset-0 bg-gradient-to-t from-black/75 via-black/35 to-black/10" />
        </div>

        {/* Centered Authentication Card */}
        <div className="absolute inset-0 flex flex-col items-center justify-center px-6">
          {auth.error && hasUrlError && (
            <div className="bg-error-subtle p-4 mb-6 rounded-xl shadow-sm">
              <p className="text-sm text-error-strong font-semibold">{t('auth.login.errorTitle')}</p>
              <p className="text-xs text-error-strong/80 mt-1">{auth.error.message}</p>
            </div>
          )}
          <h1 className="lg:hidden text-white text-2xl font-bold text-center tracking-tight mb-10 -mt-20 drop-shadow-lg">
            {systemName}
          </h1>
          <div className="bg-white/10 border border-white/15 backdrop-blur-xl py-10 px-8 xl:px-12 rounded-3xl flex flex-col xl:flex-row items-center gap-6 xl:gap-10 shadow-2xl">
            <div className="flex flex-col xl:flex-row items-center gap-8 xl:gap-12">
              <div className="flex flex-col items-center xl:items-start text-center xl:text-left">
                <h2 className="text-2xl font-bold text-white tracking-tight">{displayName}</h2>
                <p className="text-white/70 text-xs mt-1">{t('auth.login.tagline')}</p>
              </div>

              <button
                onClick={() => void auth.signinRedirect()}
                className="bg-primary hover:bg-primary-hover text-white px-10 py-3 rounded-2xl text-lg font-bold transition-all hover:scale-105 active:scale-95 shadow-lg cursor-pointer"
              >
                {t('auth.login.button')}
              </button>
            </div>
          </div>
        </div>
      </div>

      {/* Identity & Branding */}
      <div className="lg:order-first w-full lg:w-[40%] flex flex-col justify-center px-8 lg:pl-36 lg:pr-6 pt-12 pb-16 sm:pb-8 lg:pt-0 relative z-10 bg-app-surface lg:min-h-[calc(100vh-2rem)]">
        <div className="max-w-md mx-auto lg:mx-0 flex flex-col justify-center items-center lg:justify-start lg:items-start">
          {logoUrl && <img src={logoUrl} alt={appName} className="hidden lg:block h-32 mb-5 object-contain" />}

          <h1 className="hidden lg:block text-3xl font-bold text-foreground leading-tight mb-5">{systemName}</h1>

          <p className="text-md text-foreground-muted leading-relaxed text-center lg:text-left">{description}</p>

          {partnerLogos && partnerLogos.some((logo) => logo.url) && (
            <div className="flex flex-row flex-wrap items-center justify-center lg:justify-start gap-4 mt-5">
              {partnerLogos
                .filter((logo) => logo.url)
                .map((logo, index) => (
                  <img
                    key={`${logo.url}-${index}`}
                    src={logo.url}
                    alt={logo.alt}
                    className="h-10 object-contain opacity-80"
                  />
                ))}
            </div>
          )}
        </div>
      </div>
    </div>
  )
}
