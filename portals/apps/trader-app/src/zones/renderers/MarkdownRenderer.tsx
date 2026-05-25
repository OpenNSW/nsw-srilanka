import ReactMarkdown from 'react-markdown'
import type { ZoneRendererProps } from './types'

export function MarkdownRenderer({ payload }: ZoneRendererProps<'MARKDOWN'>) {
  return (
    <div className="text-sm text-gray-700 leading-relaxed space-y-3">
      <ReactMarkdown
        components={{
          h1: ({ children }) => <h1 className="text-xl font-bold text-gray-900 mt-4 mb-2">{children}</h1>,
          h2: ({ children }) => <h2 className="text-lg font-semibold text-gray-900 mt-4 mb-2">{children}</h2>,
          h3: ({ children }) => <h3 className="text-base font-semibold text-gray-900 mt-3 mb-1">{children}</h3>,
          p: ({ children }) => <p className="text-gray-700">{children}</p>,
          a: ({ children, href }) => (
            <a href={href} target="_blank" rel="noreferrer" className="text-indigo-600 hover:underline">
              {children}
            </a>
          ),
          strong: ({ children }) => <strong className="font-semibold text-gray-900">{children}</strong>,
          em: ({ children }) => <em className="italic text-gray-800">{children}</em>,
          ul: ({ children }) => <ul className="list-disc pl-5 space-y-1 text-gray-700">{children}</ul>,
          ol: ({ children }) => <ol className="list-decimal pl-5 space-y-1 text-gray-700">{children}</ol>,
          li: ({ children }) => <li>{children}</li>,
          code: ({ children }) => (
            <code className="bg-gray-100 text-pink-600 px-1.5 py-0.5 rounded text-xs font-mono">{children}</code>
          ),
          blockquote: ({ children }) => (
            <blockquote className="border-l-4 border-gray-200 pl-4 italic text-gray-600">{children}</blockquote>
          ),
        }}
      >
        {payload.content}
      </ReactMarkdown>
    </div>
  )
}
