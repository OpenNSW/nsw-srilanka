import { Link, useLocation, useNavigate } from 'react-router-dom'
import { ChevronDownIcon, HamburgerMenuIcon } from '@radix-ui/react-icons'
import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'
import { DropdownMenu } from '@radix-ui/themes'
import { useScopes } from '@/hooks/useScopes'
import { NAV_STRUCTURE, isNavGroup, permittedNav, type NavItem } from '@/components/Layout/navItems'

// The app's nav destinations the user's scopes reach, surfaced as a menu in
// the TopBar rather than a permanent side rail. onNavigate fires when the
// person picks a destination, so a menu holding this one (the TopBar's
// small-screen menu) can close.
export function NavMenu({ onNavigate }: { onNavigate?: () => void }) {
  const navigate = useNavigate()
  const location = useLocation()
  const { t } = useTranslation()
  const { hasScope } = useScopes()

  const navStructure = useMemo(() => permittedNav(NAV_STRUCTURE, hasScope), [hasScope])

  const isItemActive = (item: NavItem) =>
    location.pathname === item.path || location.pathname.startsWith(`${item.path}/`)

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
    return (
      <Link
        to={target.path}
        onClick={onNavigate}
        aria-current={isItemActive(target) ? 'page' : undefined}
        className={`flex items-center gap-2 pl-2.5 pr-3 h-10 rounded-xl text-sm font-semibold transition-colors ${
          isItemActive(target) ? 'bg-primary-subtle text-primary' : 'text-foreground hover:bg-app-surface-muted'
        }`}
      >
        <span className={isItemActive(target) ? '' : 'text-foreground-muted'}>{target.icon}</span>
        <span>{t(target.labelKey)}</span>
      </Link>
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
                    key={child.path}
                    onSelect={() => {
                      onNavigate?.()
                      void navigate(child.path)
                    }}
                    className={isItemActive(child) ? 'bg-primary-subtle! text-primary!' : undefined}
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
              key={item.path}
              onSelect={() => {
                onNavigate?.()
                void navigate(item.path)
              }}
              className={isItemActive(item) ? 'bg-primary-subtle! text-primary!' : undefined}
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
