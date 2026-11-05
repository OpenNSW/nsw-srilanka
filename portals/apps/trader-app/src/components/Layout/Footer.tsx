import { Trans, useTranslation } from 'react-i18next'
import { useMatch } from 'react-router-dom'
import { appConfig } from '@/config'
import { ROUTES } from '@/constants'

// Deliberately tiny — a thin white bar, not a full government footer. Fixed
// full-width across the bottom of the viewport (z-30, below TopBar's z-50 —
// see that component). Mounted once in App.tsx alongside <Routes>, so it's
// the same one instance across every screen.
//
// Height contract: h-16 (64px) below lg, lg:h-8 (32px) at lg+ breakpoints. Below lg the groups
// stack, since the single row only has room for the links, the "Powered by" text and the
// copyright notice from 1024px up.
// Every screen (Layout.tsx, LoginScreen.tsx,
// ZonePreviewScreen.tsx) must reserve space for it with `pb-16 lg:pb-8` so
// the reserve stays in sync with this component.
//
// Except on a task page: there the form's action bar sticks to the bottom of
// the screen, and a fixed footer would cover its buttons. The footer sits at
// the end of the page instead, below the reserve.
export function Footer() {
  const { t } = useTranslation()
  const onTaskPage = useMatch(ROUTES.TASK_FORM_PATTERN) !== null
  const footerLinks = appConfig.branding.footerLinks ?? []
  // Set only by a release image build (see the Dockerfile); a local or
  // untagged build shows no version rather than a placeholder.
  const version = import.meta.env.VITE_APP_VERSION as string | undefined
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
    <footer
      className={`${onTaskPage ? 'relative' : 'fixed inset-x-0 bottom-0'} z-30 flex h-16 flex-col items-center justify-center gap-1 bg-app-surface px-4 py-1.5 text-xs lg:h-8 lg:flex-row lg:justify-between lg:gap-4 lg:px-6 lg:py-0`}
    >
      <div className="flex flex-wrap items-center justify-center gap-x-4 gap-y-1 lg:order-1 lg:flex-nowrap lg:whitespace-nowrap">
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
      <div className="flex items-center gap-5 text-foreground lg:order-3 lg:shrink-0 lg:whitespace-nowrap lg:text-base">
        <span>
          <Trans
            i18nKey="common.poweredBy"
            components={[
              <a href="https://github.com/opennsw" target="_blank" rel="noreferrer" className="hover:underline" />,
              <a href="https://opensource.lk/" target="_blank" rel="noreferrer" className="hover:underline" />,
            ]}
          />
        </span>
        {version && <span>{version}</span>}
      </div>
      {/* Last in DOM order so it's bottom-most when the footer stacks below lg; at lg+
          it moves between the two groups and takes the space they leave, truncating rather than
          overlapping either. */}
      {copyrightNotice && (
        <span className="text-foreground-muted lg:order-2 lg:min-w-0 lg:flex-1 lg:truncate lg:text-center">
          {copyrightNotice}
        </span>
      )}
    </footer>
  )
}
