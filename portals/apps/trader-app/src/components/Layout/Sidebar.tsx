import { Link, useLocation } from 'react-router-dom'
import { DashboardIcon, ChevronDownIcon, ChevronLeftIcon, ChevronRightIcon } from '@radix-ui/react-icons'
import { type ReactNode, useEffect, useRef, useState, useMemo } from 'react'
import { useTranslation } from 'react-i18next'
import { useRole } from '@/services/useRole'
import type { Role } from '@/services/RoleContext'
import { displayName } from '@/config'

interface NavItem {
  name: string
  path: string
  icon: ReactNode
  roles?: Role[]
}

interface NavGroup {
  name: string
  icon: React.ReactNode
  items: NavItem[]
  roles?: Role[]
}

type NavItemOrGroup = NavItem | NavGroup

function isNavGroup(item: NavItemOrGroup): item is NavGroup {
  return 'items' in item
}

interface SidebarProps {
  isExpanded: boolean
  onToggle: () => void
}

export function Sidebar({ isExpanded, onToggle }: SidebarProps) {
  const location = useLocation()
  const { role } = useRole()
  const { t } = useTranslation()
  const [isHovered, setIsHovered] = useState(false)
  const [expandedGroups, setExpandedGroups] = useState<Set<string>>(new Set())
  const previousPathRef = useRef<string>(location.pathname)

  const navStructure = useMemo(
    (): NavItemOrGroup[] => [
      { name: t('sidebar.nav.consignments'), path: '/consignments', icon: <DashboardIcon className="w-5 h-5" /> },
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

  const showExpanded = isExpanded || (!isExpanded && isHovered)

  useEffect(() => {
    if (previousPathRef.current !== location.pathname) {
      previousPathRef.current = location.pathname
      const groupsToExpand = new Set<string>()
      filteredNavStructure.forEach((item) => {
        if (isNavGroup(item)) {
          const hasActivePath = item.items.some((child) => child.path === location.pathname)
          if (hasActivePath) {
            groupsToExpand.add(item.name)
          }
        }
      })
      setExpandedGroups(groupsToExpand)
    }
  }, [location.pathname, filteredNavStructure])

  const toggleGroup = (groupName: string) => {
    setExpandedGroups((prev) => {
      const next = new Set(prev)
      if (next.has(groupName)) {
        next.delete(groupName)
      } else {
        next.add(groupName)
      }
      return next
    })
  }

  const renderNavItem = (item: NavItem, isInGroup = false) => {
    const isActive = location.pathname === item.path
    return (
      <Link
        key={item.path}
        to={item.path}
        className={clsx(
          'flex items-center gap-4 px-3 h-12 min-h-12 shrink-0 rounded-xl font-medium transition-all',
          isActive
            ? 'bg-gradient-to-r from-primary to-[#8b7ff5] text-white shadow-lg shadow-primary/40'
            : 'text-white/65 hover:bg-white/10 hover:text-white',
          !showExpanded && 'justify-center',
          isInGroup && showExpanded && 'ml-4 text-sm',
        )}
        title={!showExpanded ? item.name : undefined}
      >
        <span className="flex items-center text-xl shrink-0">{item.icon}</span>
        {showExpanded && <span className="text-[15px] whitespace-nowrap">{item.name}</span>}
      </Link>
    )
  }

  const renderNavGroup = (group: NavGroup) => {
    const isGroupExpanded = expandedGroups.has(group.name)
    const hasActivePath = group.items.some((item) => item.path === location.pathname)

    if (!showExpanded) {
      return (
        <div key={group.name} className="flex flex-col gap-1">
          <div className={clsx('flex flex-col gap-1 rounded-xl transition-all', isGroupExpanded && 'bg-white/10 p-1')}>
            <button
              onClick={() => toggleGroup(group.name)}
              className={clsx(
                'relative flex items-center justify-center px-3 h-12 min-h-12 shrink-0 rounded-xl transition-all border',
                isGroupExpanded
                  ? 'text-white hover:bg-white/15 border-white/20'
                  : hasActivePath
                    ? 'bg-white/15 text-white border-white/10'
                    : 'text-white/65 hover:bg-white/10 hover:text-white border-transparent hover:border-white/10',
              )}
              title={group.name}
            >
              <span className="flex items-center text-xl shrink-0">{group.icon}</span>
              <ChevronDownIcon
                className={clsx(
                  'w-3 h-3 absolute bottom-1 right-1 transition-transform',
                  isGroupExpanded ? 'rotate-0' : '-rotate-90',
                )}
              />
            </button>

            {isGroupExpanded &&
              group.items.map((item) => {
                const isActive = location.pathname === item.path
                return (
                  <Link
                    key={item.path}
                    to={item.path}
                    className={clsx(
                      'flex items-center justify-center px-3 h-12 min-h-12 shrink-0 rounded-xl transition-all',
                      isActive
                        ? 'bg-gradient-to-r from-primary to-[#8b7ff5] text-white shadow-lg shadow-primary/40'
                        : 'text-white/65 hover:bg-white/10 hover:text-white',
                    )}
                    title={item.name}
                  >
                    <span className="flex items-center text-xl shrink-0">{item.icon}</span>
                  </Link>
                )
              })}
          </div>
        </div>
      )
    }

    return (
      <div key={group.name} className="flex flex-col gap-1">
        <button
          onClick={() => toggleGroup(group.name)}
          className={clsx(
            'flex items-center gap-4 px-3 h-12 min-h-12 shrink-0 rounded-xl font-medium transition-all w-full',
            hasActivePath && isGroupExpanded ? 'bg-white/10 text-white' : 'text-white/65 hover:bg-white/10 hover:text-white',
          )}
        >
          <span className="flex items-center text-xl shrink-0">{group.icon}</span>
          <span className="text-[15px] whitespace-nowrap flex-1 text-left">{group.name}</span>
          <ChevronDownIcon
            className={clsx('w-4 h-4 transition-transform', isGroupExpanded ? 'rotate-0' : '-rotate-90')}
          />
        </button>
        {isGroupExpanded && (
          <div className="flex flex-col gap-1">{group.items.map((item) => renderNavItem(item, true))}</div>
        )}
      </div>
    )
  }

  const brandInitial = displayName.trim().charAt(0).toUpperCase() || 'N'

  return (
    <aside
      className={`${
        showExpanded ? 'w-64' : 'w-20'
      } h-[calc(100vh-164px)] sm:h-[calc(100vh-132px)] bg-gradient-to-b from-[#2b2870] via-[#1d1a4a] to-[#100e28] text-white flex flex-col fixed left-3 top-[88px] rounded-2xl overflow-hidden border border-white/10 shadow-2xl shadow-primary-dark/40 transition-all duration-300 z-20`}
      onMouseEnter={() => !isExpanded && setIsHovered(true)}
      onMouseLeave={() => !isExpanded && setIsHovered(false)}
    >
      <div className="flex items-center gap-3 px-4 h-16 shrink-0 border-b border-white/10">
        <span className="flex h-9 w-9 shrink-0 items-center justify-center rounded-xl bg-gradient-to-br from-primary to-[#8b7ff5] text-sm font-bold text-white shadow-md shadow-primary/30">
          {brandInitial}
        </span>
        {showExpanded && <span className="text-sm font-semibold text-white truncate">{displayName}</span>}
      </div>

      <nav className="flex-1 p-3 flex flex-col gap-1 overflow-y-auto">
        {filteredNavStructure.map((item) => {
          if (isNavGroup(item)) {
            return renderNavGroup(item)
          }
          return renderNavItem(item)
        })}
      </nav>

      <div className="border-t border-white/10">
        <div className="p-4">
          <button
            onClick={onToggle}
            className={`${
              showExpanded ? 'w-full' : 'w-10'
            } h-10 rounded-full bg-gradient-to-r from-primary to-[#8b7ff5] hover:brightness-110 flex items-center ${
              showExpanded ? 'justify-between px-4' : 'justify-center'
            } text-white transition-all shadow-lg shadow-primary/30`}
            title={isExpanded ? t('sidebar.toggle.collapseTitle') : t('sidebar.toggle.expandTitle')}
          >
            {showExpanded && (
              <span className="text-sm font-medium">
                {isExpanded ? t('sidebar.toggle.collapse') : t('sidebar.toggle.expand')}
              </span>
            )}
            {isExpanded ? <ChevronLeftIcon className="w-5 h-5" /> : <ChevronRightIcon className="w-5 h-5" />}
          </button>
        </div>
      </div>
    </aside>
  )
}

function clsx(...classes: (string | boolean | undefined)[]) {
  return classes.filter(Boolean).join(' ')
}
