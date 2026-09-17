import { useNavigate, useLocation } from 'react-router-dom'
import { DashboardIcon, ChevronDownIcon, HamburgerMenuIcon } from '@radix-ui/react-icons'
import { type ReactNode, useMemo } from 'react'
import { useTranslation } from 'react-i18next'
import { DropdownMenu } from '@radix-ui/themes'
import { useRole } from '@/services/useRole'
import type { Role } from '@/services/RoleContext'

interface NavItem {
  name: string
  path: string
  icon: ReactNode
  roles?: Role[]
}

interface NavGroup {
  name: string
  icon: ReactNode
  items: NavItem[]
  roles?: Role[]
}

type NavItemOrGroup = NavItem | NavGroup

function isNavGroup(item: NavItemOrGroup): item is NavGroup {
  return 'items' in item
}

// The app's one primary nav destination, surfaced as a menu in the TopBar
// rather than a permanent side rail — kept as an items/groups structure
// (not just a single link) so a second destination or a grouped section
// slots in without a redesign, the same shape the old Sidebar used.
export function NavMenu() {
  const navigate = useNavigate()
  const location = useLocation()
  const { role } = useRole()
  const { t } = useTranslation()

  const navStructure = useMemo(
    (): NavItemOrGroup[] => [
      { name: t('nav.consignments'), path: '/consignments', icon: <DashboardIcon className="w-4 h-4" /> },
    ],
    [t],
  )

  const filteredNavStructure = useMemo(
    () =>
      navStructure
        .filter((item) => !(item.roles && !item.roles.includes(role)))
        .map((item) => {
          if (isNavGroup(item)) {
            return {
              ...item,
              items: item.items.filter((child) => !child.roles || child.roles.includes(role)),
            }
          }
          return item
        })
        .filter((item) => !(isNavGroup(item) && item.items.length === 0)),
    [role, navStructure],
  )

  const isItemActive = (item: NavItem) => location.pathname.startsWith(item.path)

  const currentLabel = useMemo(() => {
    for (const item of filteredNavStructure) {
      if (isNavGroup(item)) {
        const active = item.items.find(isItemActive)
        if (active) return active.name
      } else if (isItemActive(item)) {
        return item.name
      }
    }
    return t('nav.menu')
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [filteredNavStructure, location.pathname, t])

  return (
    <DropdownMenu.Root>
      <DropdownMenu.Trigger>
        <button className="flex items-center gap-2 pl-2.5 pr-2 h-10 rounded-xl border border-border bg-app-surface-muted/70 hover:bg-app-surface-muted text-sm font-semibold text-foreground transition-colors cursor-pointer focus:outline-none">
          <HamburgerMenuIcon className="w-4 h-4 text-foreground-subtle" />
          <span>{currentLabel}</span>
          <ChevronDownIcon className="w-3.5 h-3.5 text-foreground-subtle" />
        </button>
      </DropdownMenu.Trigger>
      <DropdownMenu.Content align="start" className="min-w-56">
        {filteredNavStructure.map((item) =>
          isNavGroup(item) ? (
            <DropdownMenu.Sub key={item.name}>
              <DropdownMenu.SubTrigger>
                <span className="flex items-center gap-2">
                  {item.icon}
                  {item.name}
                </span>
              </DropdownMenu.SubTrigger>
              <DropdownMenu.SubContent>
                {item.items.map((child) => (
                  <DropdownMenu.Item
                    key={child.path}
                    onSelect={() => void navigate(child.path)}
                    className={isItemActive(child) ? 'bg-primary-subtle! text-primary!' : undefined}
                  >
                    <span className="flex items-center gap-2">
                      {child.icon}
                      {child.name}
                    </span>
                  </DropdownMenu.Item>
                ))}
              </DropdownMenu.SubContent>
            </DropdownMenu.Sub>
          ) : (
            <DropdownMenu.Item
              key={item.path}
              onSelect={() => void navigate(item.path)}
              className={isItemActive(item) ? 'bg-primary-subtle! text-primary!' : undefined}
            >
              <span className="flex items-center gap-2">
                {item.icon}
                {item.name}
              </span>
            </DropdownMenu.Item>
          ),
        )}
      </DropdownMenu.Content>
    </DropdownMenu.Root>
  )
}
