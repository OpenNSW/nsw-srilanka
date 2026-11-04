import { Link, useLocation, useNavigate } from 'react-router-dom'
import { ChevronDownIcon, HamburgerMenuIcon } from '@radix-ui/react-icons'
import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'
import { DropdownMenu } from '@radix-ui/themes'
import { useAbilities } from '@/hooks/useCan'
import { NAV_STRUCTURE, isNavGroup, permittedNav, type NavItem } from '@/navigation/navItems'

function NavLinkRow({ item, active, onNavigate }: { item: NavItem; active: boolean; onNavigate?: () => void }) {
  const { t } = useTranslation()
  return (
    <Link
      to={item.route.path}
      onClick={onNavigate}
      aria-current={active ? 'page' : undefined}
      className={`flex items-center gap-2 pl-2.5 pr-3 h-10 rounded-xl text-sm font-semibold transition-colors ${
        active ? 'bg-primary-subtle text-primary-strong' : 'text-foreground hover:bg-app-surface-muted'
      }`}
    >
      <span className={active ? '' : 'text-foreground-muted'}>{item.icon}</span>
      <span>{t(item.labelKey)}</span>
    </Link>
  )
}

// The app's nav destinations the user may open, surfaced as a menu in
// the TopBar rather than a permanent side rail. onNavigate fires when the
// person picks a destination, so a menu holding this one (the TopBar's
// small-screen menu) can close.
//
// layout 'dropdown' is a menu button with flyout submenus for groups. 'list'
// lays every destination out as a link, a group as a labelled run of them, for
// a menu that is already open (the TopBar's small-screen menu), where a nested
// dropdown with flyouts is awkward to use by touch.
export function NavMenu({
  onNavigate,
  layout = 'dropdown',
}: {
  onNavigate?: () => void
  layout?: 'dropdown' | 'list'
}) {
  const navigate = useNavigate()
  const location = useLocation()
  const { t } = useTranslation()
  const { can } = useAbilities()

  const navStructure = useMemo(() => permittedNav(NAV_STRUCTURE, can), [can])

  const isItemActive = (item: NavItem) =>
    location.pathname === item.route.path || location.pathname.startsWith(`${item.route.path}/`)

  const currentLabel = useMemo(() => {
    for (const item of navStructure) {
      if (isNavGroup(item)) {
        const active = item.items.find(isItemActive)
        if (active) return t(active.labelKey)
      } else if (isItemActive(item)) {
        return t(item.labelKey)
      }
    }
    return t('nav.menu')
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [navStructure, location.pathname, t])

  // A dropdown that opens to reveal the one destination you're either
  // already on or about to go to is just noise — same call the app already
  // makes for LanguageSwitcher when there's only one option.
  // Show it as a plain link instead, and only promote to the full menu once
  // there's an actual choice (a second item, or any group).
  const destinationCount = navStructure.reduce((count, item) => count + (isNavGroup(item) ? item.items.length : 1), 0)
  if (destinationCount <= 1) {
    const only = navStructure[0]
    if (!only) return null
    const target = isNavGroup(only) ? only.items[0] : only
    if (!target) return null
    return <NavLinkRow item={target} active={isItemActive(target)} onNavigate={onNavigate} />
  }

  if (layout === 'list') {
    return (
      <nav aria-label={t('nav.menu')} className="flex w-full flex-col gap-1">
        {navStructure.map((item) =>
          isNavGroup(item) ? (
            <div key={item.labelKey} role="group" aria-label={t(item.labelKey)} className="flex flex-col gap-1">
              <div
                aria-hidden
                className="flex items-center gap-2 px-2.5 pt-2 text-xs font-semibold uppercase tracking-wide text-foreground-muted"
              >
                {item.icon}
                {t(item.labelKey)}
              </div>
              {item.items.map((child) => (
                <NavLinkRow key={child.route.path} item={child} active={isItemActive(child)} onNavigate={onNavigate} />
              ))}
            </div>
          ) : (
            <NavLinkRow key={item.route.path} item={item} active={isItemActive(item)} onNavigate={onNavigate} />
          ),
        )}
      </nav>
    )
  }

  return (
    <DropdownMenu.Root>
      <DropdownMenu.Trigger>
        <button className="flex items-center gap-2 pl-2.5 pr-2 h-10 rounded-xl bg-app-surface-muted/70 hover:bg-app-surface-muted text-sm font-semibold text-foreground transition-colors cursor-pointer focus:outline-none">
          <HamburgerMenuIcon className="w-4 h-4 text-foreground-muted" />
          <span>{currentLabel}</span>
          <ChevronDownIcon className="w-3.5 h-3.5 text-foreground-muted" />
        </button>
      </DropdownMenu.Trigger>
      <DropdownMenu.Content align="start" className="min-w-56">
        {navStructure.map((item) =>
          isNavGroup(item) ? (
            <DropdownMenu.Sub key={item.labelKey}>
              <DropdownMenu.SubTrigger>
                <span className="flex items-center gap-2">
                  {item.icon}
                  {t(item.labelKey)}
                </span>
              </DropdownMenu.SubTrigger>
              <DropdownMenu.SubContent>
                {item.items.map((child) => (
                  <DropdownMenu.Item
                    key={child.route.path}
                    onSelect={() => {
                      onNavigate?.()
                      void navigate(child.route.path)
                    }}
                    className={isItemActive(child) ? 'bg-primary-subtle! text-primary-strong!' : undefined}
                  >
                    <span className="flex items-center gap-2">
                      {child.icon}
                      {t(child.labelKey)}
                    </span>
                  </DropdownMenu.Item>
                ))}
              </DropdownMenu.SubContent>
            </DropdownMenu.Sub>
          ) : (
            <DropdownMenu.Item
              key={item.route.path}
              onSelect={() => {
                onNavigate?.()
                void navigate(item.route.path)
              }}
              className={isItemActive(item) ? 'bg-primary-subtle! text-primary-strong!' : undefined}
            >
              <span className="flex items-center gap-2">
                {item.icon}
                {t(item.labelKey)}
              </span>
            </DropdownMenu.Item>
          ),
        )}
      </DropdownMenu.Content>
    </DropdownMenu.Root>
  )
}
