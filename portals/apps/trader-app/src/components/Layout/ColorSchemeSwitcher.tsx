import { DropdownMenu } from '@radix-ui/themes'
import { CheckIcon } from '@radix-ui/react-icons'
import { COLOR_SCHEMES } from '@/services/colorSchemeContextCore'
import { useColorScheme } from '@/services/useColorScheme'

// A demo control for trying different brand palettes — see
// services/colorSchemeContextCore.ts for the options and index.css for
// what each one actually overrides. Lives in the footer (not the TopBar)
// since it's a whole-app setting, not a page action.
export function ColorSchemeSwitcher() {
  const { scheme, setSchemeId } = useColorScheme()

  return (
    <DropdownMenu.Root>
      <DropdownMenu.Trigger>
        <button className="flex items-center gap-1.5 px-2 py-1.5 rounded-lg text-xs text-foreground-muted hover:bg-app-surface-muted transition-colors cursor-pointer focus:outline-none">
          <span
            className="w-3 h-3 rounded-full border border-black/10 shrink-0"
            style={{ backgroundColor: scheme.swatch }}
            aria-hidden
          />
          <span>{scheme.label}</span>
        </button>
      </DropdownMenu.Trigger>
      <DropdownMenu.Content align="end" side="top" className="min-w-64">
        <DropdownMenu.Label className="text-foreground-subtle">Color scheme</DropdownMenu.Label>
        {COLOR_SCHEMES.map((s) => (
          <DropdownMenu.Item key={s.id} onSelect={() => setSchemeId(s.id)} className="items-start py-2">
            <span
              className="mt-0.5 w-3.5 h-3.5 rounded-full border border-black/10 shrink-0"
              style={{ backgroundColor: s.swatch }}
              aria-hidden
            />
            <span className="flex-1 min-w-0">
              <span className="flex items-center gap-1.5">
                <span className="font-medium">{s.label}</span>
                {s.id === scheme.id && <CheckIcon className="w-3.5 h-3.5 text-primary" />}
              </span>
              <span className="block text-xs text-foreground-subtle leading-snug">{s.description}</span>
            </span>
          </DropdownMenu.Item>
        ))}
      </DropdownMenu.Content>
    </DropdownMenu.Root>
  )
}
