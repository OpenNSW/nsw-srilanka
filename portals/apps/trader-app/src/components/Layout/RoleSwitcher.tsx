import { BackpackIcon, IdCardIcon } from '@radix-ui/react-icons'
import { Select, Flex, Text, Box } from '@radix-ui/themes'
import { type ReactNode } from 'react'
import { useRole, type Role } from '../../services/RoleContext'

const ROLE_CONFIG: Record<
  Role,
  {
    label: string
    description: string
    dropdownDescription: string
    icon: ReactNode
  }
> = {
  trader: {
    label: 'Trader',
    description: 'Managing consignments',
    dropdownDescription: 'Create and manage consignments',
    icon: <BackpackIcon className="text-info-strong" />,
  },
  cha: {
    label: 'CHA',
    description: 'Handling Customs Clearances',
    dropdownDescription: 'Handle customs clearances',
    icon: <IdCardIcon className="text-warning-strong" />,
  },
}

function RoleDisplay({ role, showPrimaryLabel }: { role: Role; showPrimaryLabel: boolean }) {
  const { label, description, icon } = ROLE_CONFIG[role]

  return (
    <Flex align="center" gap="3" className="w-60 text-left">
      <Box className="rounded-md border border-border bg-background p-1.5 shadow-sm">{icon}</Box>
      <Box className="flex-1">
        <Flex align="center" gap="1">
          <Text size="1" weight="bold" className="block leading-none">
            {label}
          </Text>
          {showPrimaryLabel && (
            <Text size="1" color="gray" className="font-normal">
              (Primary)
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

  const showSwitcher = availableRoles.length > 1

  return (
    <Box className="flex-1 max-w-md px-8">
      {!isLoading ? (
        <Box className="h-full w-full">
          <Select.Root value={role} onValueChange={(val) => setRole(val as Role)} disabled={!showSwitcher}>
            <Select.Trigger
              variant="ghost"
              className={`h-12 w-full p-4 transition-all ${showSwitcher ? 'cursor-pointer hover:bg-surface-muted' : 'cursor-default'}`}
            >
              <RoleDisplay role={role} showPrimaryLabel={!showSwitcher} />
            </Select.Trigger>

            {showSwitcher && (
              <Select.Content position="popper" className="w-full min-w-[320px]">
                {availableRoles.map((r) => {
                  const { label, dropdownDescription, icon } = ROLE_CONFIG[r]

                  return (
                    <Select.Item
                      key={r}
                      value={r}
                      className="cursor-pointer border-none py-2 transition-colors focus:bg-surface-muted data-highlighted:bg-surface-muted! data-highlighted:text-inherit!"
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
        <Box className="h-12 w-full animate-pulse rounded-lg border border-border bg-surface" />
      )}
    </Box>
  )
}
