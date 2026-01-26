import { Card, Text } from '@radix-ui/themes'
import type {Workflow} from "../../services/types/workflow.ts";

interface WorkflowCardProps {
  workflow: Workflow
  selected: boolean
  onSelect: (workflow: Workflow) => void
}

export function WorkflowCard({ workflow, selected, onSelect }: WorkflowCardProps) {
  return (
    <Card
      className={`cursor-pointer transition-all ${
        selected
          ? 'ring-2 ring-blue-500 bg-blue-50'
          : 'hover:bg-gray-50'
      }`}
      onClick={() => onSelect(workflow)}
    >
      <Text as="div" size="2" weight="bold">
        {workflow.name}
      </Text>
      <Text as="div" size="1" color="gray" mt="1">
        {workflow.description}
      </Text>
    </Card>
  )
}