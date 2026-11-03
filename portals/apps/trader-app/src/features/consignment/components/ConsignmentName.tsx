import { Text } from '@radix-ui/themes'

// A consignment or case in a list: its name over its ID, or the ID alone
// when it has no name.
export function ConsignmentName({ name, id }: { name?: string; id: string }) {
  if (!name) {
    return (
      <Text size="2" weight="medium" className="text-info-strong font-mono">
        {id}
      </Text>
    )
  }
  return (
    <div className="flex flex-col">
      <Text size="2" weight="bold" className="text-info-strong">
        {name}
      </Text>
      <Text size="1" color="gray" className="font-mono mt-0.5">
        {id}
      </Text>
    </div>
  )
}
