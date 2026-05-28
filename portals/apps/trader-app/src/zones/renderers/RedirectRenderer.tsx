import { useEffect, useState } from 'react'
import ReactMarkdown from 'react-markdown'
import { Button } from '@radix-ui/themes'
import type { ZoneRendererProps } from './types'

export function RedirectRenderer({ payload }: ZoneRendererProps<'REDIRECT'>) {
  const { checkout_url, content } = payload
  const sessionKey = `nsw:redirected:${checkout_url}`

  const [hasRedirected, setHasRedirected] = useState(() => {
    try {
      return sessionStorage.getItem(sessionKey) === 'true'
    } catch {
      return false
    }
  })

  useEffect(() => {
    if (checkout_url && !hasRedirected) {
      let success = false
      try {
        sessionStorage.setItem(sessionKey, 'true')
        success = true
      } catch (err) {
        console.error('Failed to set sessionStorage:', err)
      }
      if (success && (checkout_url.startsWith('https://') || checkout_url.startsWith('http://'))) {
        window.location.href = checkout_url
      } else {
        // Fallback: if sessionStorage is unavailable, do not auto-redirect
        // to prevent infinite loops. Force manual redirection instead.
        setHasRedirected(true)
      }
    }
  }, [checkout_url, hasRedirected, sessionKey])

  const handleGoToSession = () => {
    if (checkout_url && (checkout_url.startsWith('https://') || checkout_url.startsWith('http://'))) {
      window.location.href = checkout_url
    }
  }

  const handleReset = () => {
    try {
      sessionStorage.removeItem(sessionKey)
    } catch (err) {
      console.error('Failed to remove sessionStorage key:', err)
    }
    setHasRedirected(false)
  }

  if (hasRedirected) {
    return (
      <div className="rounded-lg border border-indigo-200 bg-indigo-50/40 p-6 text-sm text-gray-700 shadow-sm transition-all duration-300">
        <div className="flex items-center gap-3 mb-4">
          <div className="p-2 bg-indigo-100 rounded-full text-indigo-700">
            <svg
              xmlns="http://www.w3.org/2000/svg"
              className="w-5 h-5"
              fill="none"
              viewBox="0 0 24 24"
              stroke="currentColor"
              strokeWidth={2}
            >
              <path
                strokeLinecap="round"
                strokeLinejoin="round"
                d="M10 6H6a2 2 0 00-2 2v10a2 2 0 002 2h10a2 2 0 002-2v-4M14 4h6m0 0v6m0-6L10 14"
              />
            </svg>
          </div>
          <div>
            <h4 className="font-semibold text-indigo-900 text-base">Redirected to Payment Gateway</h4>
            <p className="text-xs text-gray-500">Your session was redirected to the external payment gateway.</p>
          </div>
        </div>

        <div className="text-gray-600 mb-6 bg-white/60 p-4 rounded-md border border-gray-100">
          <ReactMarkdown
            components={{
              a: ({ children, href }) => (
                <a href={href} target="_blank" rel="noreferrer" className="text-indigo-600 hover:underline">
                  {children}
                </a>
              ),
              p: ({ children }) => <p className="text-gray-700 leading-relaxed mb-2 last:mb-0">{children}</p>,
              strong: ({ children }) => <strong className="font-semibold text-gray-900">{children}</strong>,
              em: ({ children }) => <em className="italic text-gray-800">{children}</em>,
              h3: ({ children }) => <h3 className="text-base font-semibold text-gray-900 mt-2 mb-1">{children}</h3>,
            }}
          >
            {content}
          </ReactMarkdown>
        </div>

        <div className="flex items-center gap-3">
          <Button
            type="button"
            size="3"
            onClick={handleGoToSession}
            className="cursor-pointer font-medium shadow-sm flex items-center gap-2"
          >
            Return to payment session
            <svg
              xmlns="http://www.w3.org/2000/svg"
              className="w-4 h-4"
              fill="none"
              viewBox="0 0 24 24"
              stroke="currentColor"
              strokeWidth={2}
            >
              <path strokeLinecap="round" strokeLinejoin="round" d="M14 5l7 7m0 0l-7 7m7-7H3" />
            </svg>
          </Button>

          <Button
            type="button"
            variant="ghost"
            color="gray"
            size="3"
            onClick={handleReset}
            className="cursor-pointer text-gray-500 hover:text-gray-900 transition-colors"
          >
            Reset redirection state
          </Button>
        </div>
      </div>
    )
  }

  return (
    <div className="rounded border border-indigo-200 bg-indigo-50/40 p-6 text-sm text-gray-700">
      <p className="mb-3 font-medium text-indigo-700">Redirecting to payment gateway…</p>
      <ReactMarkdown
        components={{
          a: ({ children, href }) => (
            <a href={href} target="_blank" rel="noreferrer" className="text-indigo-600 hover:underline">
              {children}
            </a>
          ),
          p: ({ children }) => <p className="text-gray-700">{children}</p>,
          strong: ({ children }) => <strong className="font-semibold text-gray-900">{children}</strong>,
          em: ({ children }) => <em className="italic text-gray-800">{children}</em>,
          h3: ({ children }) => <h3 className="text-base font-semibold text-gray-900 mt-2 mb-1">{children}</h3>,
        }}
      >
        {content}
      </ReactMarkdown>
    </div>
  )
}
