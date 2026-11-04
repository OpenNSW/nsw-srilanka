import { Text } from '@radix-ui/themes'

// A consignment or case in a list: its name over its ID, or the ID alone
// when it has no name. Each line ends in an ellipsis when its container is
// too narrow for it, as on a card on a phone.
export function ConsignmentName({ name, id }: { name?: string; id: string }) {
  if (!name) {
    return (
      <Text as="p" size="2" weight="medium" className="truncate text-info-strong font-mono">
        {id}
      </Text>
    )
  }
  return (
    <div className="flex min-w-0 flex-col">
      <Text size="2" weight="bold" className="truncate text-info-strong">
        {name}
      </Text>
      <Text size="1" color="gray" className="truncate font-mono mt-0.5">
        {id}
      </Text>
    </div>
  )
}
