import { useTranslation } from 'react-i18next'
import { DesktopIcon, MoonIcon, SunIcon } from '@radix-ui/react-icons'
import type { AppearanceSetting } from '@portals/theme'
import { useAppearance } from '@/services/useAppearance'

const ICONS = { system: DesktopIcon, light: SunIcon, dark: MoonIcon }

// Each press moves to the next: system, light, dark, then system again.
const NEXT: Record<AppearanceSetting, AppearanceSetting> = { system: 'light', light: 'dark', dark: 'system' }

// One button showing the current appearance; its name and tooltip say which
// it is. Styled to sit next to the LanguageSwitcher, wherever that appears.
export function AppearanceSwitcher() {
  const { t } = useTranslation()
  const { setting, setSetting } = useAppearance()
  const Icon = ICONS[setting]
  const label = t('appearance.label', { current: t(`appearance.${setting}`) })

  return (
    <button
      type="button"
      onClick={() => setSetting(NEXT[setting])}
      aria-label={label}
      title={label}
      className="flex items-center px-2 py-1.5 rounded-lg text-foreground-muted hover:bg-app-surface-muted transition-colors cursor-pointer focus:outline-none focus-visible:ring-2 focus-visible:ring-primary"
    >
      <Icon className="w-4 h-4" />
    </button>
  )
}
