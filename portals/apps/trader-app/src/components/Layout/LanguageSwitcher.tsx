import { useTranslation } from 'react-i18next'
import { DropdownMenu } from '@radix-ui/themes'
import { GlobeIcon } from '@radix-ui/react-icons'
import { supportedLanguages, languageLabels } from '@/i18n'

// Hidden entirely when only one language is bundled (see i18n/index.ts) — a
// switcher with a single option is just clutter.
export function LanguageSwitcher() {
  const { i18n } = useTranslation()

  if (supportedLanguages.length <= 1) {
    return null
  }

  const activeLanguage = i18n.resolvedLanguage ?? i18n.language
  const currentLabel = languageLabels[activeLanguage] ?? activeLanguage

  return (
    <DropdownMenu.Root>
      <DropdownMenu.Trigger>
        <button className="flex items-center gap-1.5 px-2 py-1.5 rounded-lg text-sm text-foreground-muted hover:bg-app-surface-muted transition-colors cursor-pointer focus:outline-none">
          <GlobeIcon className="w-4 h-4" />
          <span>{currentLabel}</span>
        </button>
      </DropdownMenu.Trigger>
      <DropdownMenu.Content align="end" size="2">
        <DropdownMenu.RadioGroup
          value={activeLanguage}
          onValueChange={(code) => {
            void i18n.changeLanguage(code)
          }}
        >
          {supportedLanguages.map((code) => (
            <DropdownMenu.RadioItem key={code} value={code} style={{ cursor: 'pointer' }}>
              {languageLabels[code] ?? code}
            </DropdownMenu.RadioItem>
          ))}
        </DropdownMenu.RadioGroup>
      </DropdownMenu.Content>
    </DropdownMenu.Root>
  )
}
