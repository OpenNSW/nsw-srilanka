import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import './index.css'
import './i18n'
import App from './App.tsx'
import '@radix-ui/themes/styles.css'
import { createBrowserRouter, RouterProvider } from 'react-router-dom'
import { ErrorBoundary } from '@portals/ui'
import { ErrorFallback } from './components/ErrorFallback'
import { TranslatedUiLabels } from './components/TranslatedUiLabels'
import { logger } from './utils/logger'
import { AuthProvider } from 'react-oidc-context'
import { userManager } from './oidcUserManager'
import { initAppConfig } from './config'
import { ColorSchemeProvider } from './services/ColorSchemeContext'
import { ThemedShell } from './components/ThemedShell'

initAppConfig()
  .then(() => {
    // A data router, not <BrowserRouter>: useBlocker (the unsaved-form guard)
    // only works under one. App keeps its own <Routes>, under a single splat.
    const router = createBrowserRouter([{ path: '*', element: <App /> }])
    createRoot(document.getElementById('root')!).render(
      <StrictMode>
        <ErrorBoundary
          fallback={(error, reset) => <ErrorFallback error={error} onReset={reset} />}
          onError={(error, errorInfo) => logger.error('Error boundary caught an error:', error, errorInfo)}
        >
          <AuthProvider
            userManager={userManager}
            onSigninCallback={() => {
              window.history.replaceState({}, document.title, window.location.pathname)
            }}
          >
            <ColorSchemeProvider>
              <ThemedShell>
                <TranslatedUiLabels>
                  <RouterProvider router={router} />
                </TranslatedUiLabels>
              </ThemedShell>
            </ColorSchemeProvider>
          </AuthProvider>
        </ErrorBoundary>
      </StrictMode>,
    )
  })
  .catch((err) => {
    console.error('Failed to initialize configuration:', err)
  })
