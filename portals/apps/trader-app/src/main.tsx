import {StrictMode} from 'react'
import {createRoot} from 'react-dom/client'
import './index.css'
import App from './App.tsx'
import "@radix-ui/themes/styles.css";
import {BrowserRouter} from 'react-router-dom';
import {Theme} from '@radix-ui/themes';
import { ErrorBoundary } from './components/ErrorBoundary'
import { AsgardeoProvider } from '@asgardeo/react'
import { getEnv } from './runtimeConfig'

const normalizeIdpPlatform = (value: string): 'AsgardeoV2' | 'Asgardeo' | 'IdentityServer' | 'Unknown' => {
  if (value === 'AsgardeoV2' || value === 'Asgardeo' || value === 'IdentityServer' || value === 'Unknown') {
    return value
  }

  return 'AsgardeoV2'
}

const APP_URL = getEnv('VITE_APP_URL', window.location.origin)!
const CLIENT_ID = getEnv('VITE_IDP_CLIENT_ID', 'TRADER_PORTAL_APP')!
const IDP_BASE_URL = getEnv('VITE_IDP_BASE_URL', 'https://localhost:8090')!
const IDP_PLATFORM = normalizeIdpPlatform(getEnv('VITE_IDP_PLATFORM', 'AsgardeoV2')!)
const rawScopes = getEnv('VITE_IDP_SCOPES')
const IDP_SCOPES = rawScopes
  ? rawScopes.split(',').map((s: string) => s.trim())
  : ["openid", "profile", "email"]

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <ErrorBoundary>
      <AsgardeoProvider
        clientId={CLIENT_ID}
        baseUrl={IDP_BASE_URL}
        platform={IDP_PLATFORM}
        afterSignInUrl={APP_URL}
        afterSignOutUrl={APP_URL}
        scopes={IDP_SCOPES}
      >
        <Theme>
          <BrowserRouter>
            <App/>
          </BrowserRouter>
        </Theme>
      </AsgardeoProvider>
    </ErrorBoundary>
  </StrictMode>,
)
