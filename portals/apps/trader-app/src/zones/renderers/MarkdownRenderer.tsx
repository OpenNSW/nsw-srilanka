import type { MouseEvent } from 'react'
import ReactMarkdown from 'react-markdown'
import { useUpload } from '@opennsw/jsonforms-renderers'
import type { ZoneRendererProps } from './types'

export function MarkdownRenderer({ payload }: ZoneRendererProps<'MARKDOWN'>) {
  const uploadCtx = useUpload()

  return (
    <div className="p-6 text-sm text-foreground-muted leading-relaxed space-y-3">
      <ReactMarkdown
        components={{
          h1: ({ children }) => <h1 className="text-xl font-bold text-foreground mt-4 mb-2">{children}</h1>,
          h2: ({ children }) => <h2 className="text-lg font-semibold text-foreground mt-4 mb-2">{children}</h2>,
          h3: ({ children }) => <h3 className="text-base font-semibold text-foreground mt-3 mb-1">{children}</h3>,
          p: ({ children }) => <p className="text-foreground-muted">{children}</p>,
          a: ({ children, href }) => {
            const isStorageKey =
              /^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}(\.[a-zA-Z0-9]+)?$/.test(
                href ?? '',
              )

             if (isStorageKey && href) {
              const handleClick = (e: MouseEvent<HTMLAnchorElement>) => {
                e.preventDefault()
                if (uploadCtx?.getDownloadUrl) {
                  // Synchronously open a new blank window to bypass browser popup blockers
                  const newWindow = window.open('about:blank', '_blank', 'noopener,noreferrer')
                  
                  uploadCtx
                    .getDownloadUrl(href)
                    .then(({ url }) => {
                      if (newWindow) {
                        newWindow.location.href = url
                      }
                    })
                    .catch((err) => {
                      console.error('Failed to resolve secure download url from context', err)
                      if (newWindow) {
                        newWindow.close()
                      }
                    })
                }
              }

              return (
                <a href={`/storage/${href}`} onClick={handleClick} className="text-primary hover:underline cursor-pointer">
                  {children}
                </a>
              )
            }

            return (
              <a href={href} target="_blank" rel="noreferrer" className="text-primary hover:underline">
                {children}
              </a>
            )
          },
          strong: ({ children }) => <strong className="font-semibold text-foreground">{children}</strong>,
          em: ({ children }) => <em className="italic text-foreground">{children}</em>,
          ul: ({ children }) => <ul className="list-disc pl-5 space-y-1 text-foreground-muted">{children}</ul>,
          ol: ({ children }) => <ol className="list-decimal pl-5 space-y-1 text-foreground-muted">{children}</ol>,
          li: ({ children }) => <li>{children}</li>,
          code: ({ children }) => (
            <code className="bg-surface-muted text-primary px-1.5 py-0.5 rounded text-xs font-mono">{children}</code>
          ),
          blockquote: ({ children }) => (
            <blockquote className="border-l-4 border-border pl-4 italic text-foreground-muted">{children}</blockquote>
          ),
        }}
      >
        {payload.content}
      </ReactMarkdown>
    </div>
  )
}
