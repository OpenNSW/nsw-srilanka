import { cx } from '../cx'
import { CopyButton } from './CopyButton'

// A block of preformatted text, such as JSON, with a button in its top-right
// corner that copies the whole text. The button stays in the corner while the
// text scrolls; callers set the block's height through className.
export function CodeBlock({ label, children, className }: { label: string; children: string; className?: string }) {
  return (
    <div className="relative">
      <pre
        className={cx(
          'bg-app-surface-muted rounded p-3 pr-10 text-xs font-mono overflow-auto whitespace-pre-wrap break-all',
          className,
        )}
      >
        {children}
      </pre>
      <div className="absolute top-3 right-3 flex">
        <CopyButton value={children} label={label} />
      </div>
    </div>
  )
}
