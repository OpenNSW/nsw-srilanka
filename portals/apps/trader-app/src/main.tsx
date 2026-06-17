import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import './index.css'
import './i18n'
import App from './App.tsx'
import '@radix-ui/themes/styles.css'
import { BrowserRouter } from 'react-router-dom'
import { Theme } from '@radix-ui/themes'
import { ErrorBoundary } from './components/ErrorBoundary'
import { AuthProvider } from 'react-oidc-context'
import { userManager } from './oidcUserManager'
import { initAppConfig } from './config'

initAppConfig()
  .then(() => {
    createRoot(document.getElementById('root')!).render(
      <StrictMode>
        <ErrorBoundary>
          <AuthProvider
            userManager={userManager}
            onSigninCallback={() => {
              window.history.replaceState({}, document.title, window.location.pathname)
            }}
          >
            <Theme accentColor="indigo" grayColor="slate" radius="medium" panelBackground="solid" appearance="light">
              <BrowserRouter>
                <App />
              </BrowserRouter>
            </Theme>
          </AuthProvider>
        </ErrorBoundary>
      </StrictMode>,
    )
  })
  .catch((err) => {
    console.error('Failed to initialize configuration:', err)
  })
