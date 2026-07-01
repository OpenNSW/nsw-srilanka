import { useEffect, useMemo } from 'react'
import { useAuth } from 'react-oidc-context'
import { useTranslation } from 'react-i18next'
import { appConfig, displayName } from '@/config'

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
      {/* Mobile logo strip — full-width box at the very top, hidden on desktop */}
      {logoUrl && (
        <div className="lg:hidden w-full bg-app-surface px-6 py-4 flex items-center justify-center border-b border-border shadow-sm">
          <img src={logoUrl} alt={appName} className="h-16 object-contain" />
        </div>
      )}

      {/* Hero & Authentication */}
      <div className="lg:order-last relative flex-1 min-h-125 lg:min-h-screen overflow-hidden">
        {/* Hero background — no clip on mobile (logo has its own strip above), diagonal on desktop */}
        <div className="absolute inset-0 [clip-path:none] lg:[clip-path:polygon(25%_0,100%_0,100%_100%,0%_100%)]">
          <div
            className="absolute inset-0 bg-cover bg-center bg-foreground"
            style={heroImageUrl ? { backgroundImage: `url(${heroImageUrl})` } : undefined}
          />
          <div className="absolute inset-0 bg-black/50" />
        </div>

        {/* Centered Authentication Card */}
        <div className="absolute inset-0 flex flex-col items-center justify-center px-6">
          {auth.error && hasUrlError && (
            <div className="bg-red-50 border-l-4 border-red-500 p-4 mb-6 rounded-r-xl">
              <p className="text-sm text-red-700 font-semibold">{t('auth.login.errorTitle')}</p>
              <p className="text-xs text-red-600 mt-1">{auth.error.message}</p>
            </div>
          )}
          <h1 className="lg:hidden text-white text-2xl font-bold text-center tracking-wide mb-10 -mt-20 drop-shadow-lg">
            {systemName}
          </h1>
          <div className="bg-foreground/80 border-2 border-white/30 py-10 px-8 xl:px-12 rounded-2xl flex flex-col xl:flex-row items-center gap-6 xl:gap-10 shadow-[0_20px_50px_rgba(0,0,0,0.5)]">
            <div className="flex flex-col xl:flex-row items-center gap-8 xl:gap-12">
              <div className="flex flex-col items-center xl:items-start text-center xl:text-left">
                <h2 className="text-2xl font-bold text-white tracking-wide">{displayName}</h2>
                <p className="text-white/60 text-xs mt-1">{t('auth.login.tagline')}</p>
              </div>

              <button
                onClick={() => void auth.signinRedirect()}
                className="bg-primary hover:bg-primary-dark text-white px-10 py-2.5 rounded-2xl text-lg font-bold transition-all hover:scale-105 active:scale-95 shadow-lg cursor-pointer"
              >
                {t('auth.login.button')}
              </button>
            </div>
          </div>
        </div>
      </div>

      {/* Identity & Branding */}
      <div className="lg:order-first w-full lg:w-[40%] flex flex-col justify-center px-8 lg:pl-36 lg:pr-6 py-12 lg:py-0 relative z-10 bg-app-surface lg:min-h-screen">
        <p className="absolute bottom-4 left-8 lg:left-36 text-xs text-foreground-muted">
          {import.meta.env.VITE_APP_VERSION || 'dev'}
        </p>
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
