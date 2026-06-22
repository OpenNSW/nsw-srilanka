import { BackpackIcon, IdCardIcon } from '@radix-ui/react-icons'
import { Select, Flex, Text, Box } from '@radix-ui/themes'
import { type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { useRole } from '@/services/useRole'
import type { Role } from '@/services/RoleContext'

const ROLE_ICONS: Record<Role, ReactNode> = {
  trader: <BackpackIcon className="text-info-strong" />,
  cha: <IdCardIcon className="text-warning-strong" />,
}

function RoleDisplay({ role, showPrimaryLabel }: { role: Role; showPrimaryLabel: boolean }) {
  const { t } = useTranslation()
  const icon = ROLE_ICONS[role]
  const label = t(`roles.${role}.label` as const)
  const description = t(`roles.${role}.description` as const)

  return (
    <Flex align="center" gap="3" className="w-60 text-left">
      <Box className="rounded-md border border-border bg-app-bg p-1.5 shadow-sm">{icon}</Box>
      <Box className="flex-1">
        <Flex align="center" gap="1">
          <Text size="1" weight="bold" className="block leading-none">
            {label}
          </Text>
          {showPrimaryLabel && (
            <Text size="1" color="gray" className="font-normal">
              {t('roles.primary')}
            </Text>
          )}
        </Flex>
        <Text size="1" color="gray" className="mt-0.5 block leading-tight">
          {description}
        </Text>
      </Box>
    </Flex>
  )
}

export function RoleSwitcher() {
  const { role, setRole, availableRoles, isLoading } = useRole()
  const { t } = useTranslation()

  const showSwitcher = availableRoles.length > 1

  return (
    <Box className="flex-1 max-w-md px-8">
      {!isLoading ? (
        <Box className="h-full w-full">
          <Select.Root value={role} onValueChange={(val) => setRole(val as Role)} disabled={!showSwitcher}>
            <Select.Trigger
              variant="ghost"
              className={`h-12 w-full p-4 transition-all ${showSwitcher ? 'cursor-pointer hover:bg-app-surface-muted' : 'cursor-default'}`}
            >
              <RoleDisplay role={role} showPrimaryLabel={!showSwitcher} />
            </Select.Trigger>

            {showSwitcher && (
              <Select.Content position="popper" className="w-full min-w-[320px]">
                {availableRoles.map((r) => {
                  const icon = ROLE_ICONS[r]
                  const label = t(`roles.${r}.label` as const)
                  const dropdownDescription = t(`roles.${r}.dropdownDescription` as const)

                  return (
                    <Select.Item
                      key={r}
                      value={r}
                      className="cursor-pointer border-none py-2 transition-colors focus:bg-app-surface-muted data-highlighted:bg-app-surface-muted! data-highlighted:text-inherit!"
                    >
                      <Flex direction="column" py="1">
                        <Flex align="center" gap="2">
                          {icon}
                          <Text weight="bold" size="1" className="text-foreground">
                            {label}
                          </Text>
                        </Flex>
                        <Text size="1" color="gray">
                          {dropdownDescription}
                        </Text>
                      </Flex>
                    </Select.Item>
                  )
                })}
              </Select.Content>
            )}
          </Select.Root>
        </Box>
      ) : (
        <Box className="h-12 w-full animate-pulse rounded-lg border border-border bg-app-surface" />
      )}
    </Box>
  )
}
