import { useRef } from 'react'

// A plain <textarea> with a synced line-number gutter, standing in for Radix's TextArea only in
// the variables patch editor (see ResolveAdminInterventionView) — Radix's own component doesn't expose
// the scroll position a gutter needs to stay in sync. Wrapping is deliberately off (wrap="off" +
// white-space: pre + horizontal scroll) rather than left to wrap: with it on, a long line's
// wrapped continuation would visually sit under whichever number happens to be next, since a
// gutter line only ever corresponds to one real line. label is the textarea's accessible name: the
// visible title above it (see Section) is not associated with it, so a screen reader would
// otherwise announce an unnamed edit field.
export function LineNumberedTextArea({
  value,
  onChange,
  label,
}: {
  value: string
  onChange: (value: string) => void
  label: string
}) {
  const textareaRef = useRef<HTMLTextAreaElement>(null)
  const gutterRef = useRef<HTMLDivElement>(null)
  const lineCount = value.split('\n').length

  const syncGutterScroll = () => {
    if (textareaRef.current && gutterRef.current) {
      gutterRef.current.scrollTop = textareaRef.current.scrollTop
    }
  }

  return (
    // The border lives on this wrapper, not the <textarea> — a bare <textarea> carries its own
    // UA-default border independent of any wrapper border, which without an explicit border-0
    // shows through as a second, darker (often black) border nested just inside this one.
    <div className="flex border border-border rounded overflow-hidden font-mono text-xs h-48 focus-within:border-primary">
      <div
        ref={gutterRef}
        className="shrink-0 w-9 overflow-hidden bg-app-surface-muted text-right py-2 pr-2 text-foreground-muted select-none"
        aria-hidden
      >
        {Array.from({ length: lineCount }, (_, i) => (
          <div key={i} className="leading-5">
            {i + 1}
          </div>
        ))}
      </div>
      <textarea
        ref={textareaRef}
        aria-label={label}
        value={value}
        onChange={(e) => onChange(e.target.value)}
        onScroll={syncGutterScroll}
        spellCheck={false}
        wrap="off"
        className="flex-1 min-w-0 resize-none border-0 p-2 leading-5 outline-none whitespace-pre overflow-auto bg-transparent"
      />
    </div>
  )
}
