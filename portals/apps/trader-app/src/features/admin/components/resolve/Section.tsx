import type { ReactNode } from 'react'
import { Text } from '@radix-ui/themes'

// One titled block of the resolve screen, with an optional grey note under the title. It owns the
// spacing (the gap to the next block, and title to content) on wrapper divs on purpose: a margin
// class on a Radix <Text> is not applied — its own margin reset wins — so relying on `mb-*` there
// left blocks with a text placeholder touching the block after them.
export function Section({ title, note, children }: { title: string; note?: string; children: ReactNode }) {
  return (
    <div className="mb-4">
      <div className="mb-1">
        <Text size="2" weight="medium" className="block">
          {title}
        </Text>
        {note && (
          <Text size="1" color="gray" className="block">
            {note}
          </Text>
        )}
      </div>
      {children}
    </div>
  )
}
