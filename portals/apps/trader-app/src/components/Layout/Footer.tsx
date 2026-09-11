import { useTranslation } from 'react-i18next'
import { appConfig } from '@/config'

// Deliberately tiny — a thin white bar, not a full government footer. Fixed
// full-width across the bottom of the viewport (z-30: above Sidebar's z-20,
// below TopBar's z-50 — see those components), so it's the same one instance
// regardless of what's rendered above it — see Layout.tsx and LoginScreen.tsx.
export function Footer() {
  const { t } = useTranslation()
  const footerLinks = appConfig.branding.footerLinks ?? []
  const version = (import.meta.env.VITE_APP_VERSION as string | undefined) || 'dev'
  const copyrightNotice = appConfig.branding.copyrightNotice

  // Only a footerLinks entry whose key matches one of this fixed, known set
  // is rendered, with its label translated here rather than carried in
  // branding.json (see configs/types.ts). url is always an absolute URL —
  // these are not routes in this app.
  function footerLinkLabel(key: string): string | null {
    switch (key) {
      case 'policy':
        return t('footer.links.policy')
      case 'accessibility':
        return t('footer.links.accessibility')
      case 'support':
        return t('footer.links.support')
      default:
        return null
    }
  }

  return (
    <footer className="fixed inset-x-0 bottom-0 z-30 flex flex-col items-center justify-center gap-1 border-t border-border bg-app-surface px-4 py-1.5 text-xs sm:h-8 sm:flex-row sm:justify-between sm:gap-4 sm:px-6 sm:py-0">
      <div className="flex flex-wrap items-center justify-center gap-x-4 gap-y-1">
        {footerLinks.map((link) => {
          const label = footerLinkLabel(link.key)
          if (!label) return null

          return (
            <a
              key={link.key}
              href={link.url}
              target="_blank"
              rel="noreferrer"
              className="text-foreground-muted hover:text-foreground hover:underline"
            >
              {label}
            </a>
          )
        })}
      </div>
      <div className="flex items-center gap-4 text-foreground">
        <a href="https://github.com/OpenNSW" target="_blank" rel="noreferrer" className="hover:underline">
          {t('common.poweredBy')}
        </a>
        <span>{version}</span>
      </div>
      {/* Last in DOM order so it's bottom-most when the footer stacks on narrow screens; at sm+
          it's taken out of the row's flow and centered independently of the two groups above. */}
      {copyrightNotice && (
        <span className="text-foreground-muted sm:absolute sm:left-1/2 sm:top-1/2 sm:max-w-[40%] sm:-translate-x-1/2 sm:-translate-y-1/2 sm:truncate">
          {copyrightNotice}
        </span>
      )}
    </footer>
  )
}
