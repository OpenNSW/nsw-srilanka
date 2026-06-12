import { type ReactNode } from 'react'
import { Routes, Route, Navigate } from 'react-router-dom'
import './App.css'
import { Layout } from './components/Layout'
import { ConsignmentScreen } from './screens/ConsignmentScreen.tsx'
import { ConsignmentDetailScreen } from './screens/ConsignmentDetailScreen.tsx'
import { TaskDetailScreen } from './screens/TaskDetailScreen.tsx'
import { PreconsignmentScreen } from './screens/PreconsignmentScreen.tsx'
import { useAuth } from 'react-oidc-context'
import { LoginScreen } from './screens/LoginScreen.tsx'
import { ApiProvider, useApi } from './services/ApiContext'
import { RoleProvider } from './services/RoleContext'
import { UploadProvider } from '@opennsw/jsonforms-renderers'
import { uploadFile, getDownloadUrl } from './services/storage'
import { useAuthContext } from './hooks/useAuthContext'
import { UnauthorizedScreen } from './screens/UnauthorizedScreen.tsx'
import { ZonePreviewScreen } from './screens/ZonePreviewScreen.tsx'
import { appConfig, displayName } from './config'
import { useEffect } from 'react'

function UploadWrapper({ children }: { children: ReactNode }) {
  const api = useApi()
  return (
    <UploadProvider onUpload={(file) => uploadFile(api, file)} getDownloadUrl={(key) => getDownloadUrl(api, key)}>
      {children}
    </UploadProvider>
  )
}

function ProtectedLayout() {
  const { isSignedIn, isLoading, availableRoles, isResolvingRoles } = useAuthContext()

  if (isLoading || (isSignedIn && (isResolvingRoles || availableRoles === null))) return null
  if (!isSignedIn) return <Navigate to="/login" replace />
  if (!availableRoles || availableRoles.length === 0) return <UnauthorizedScreen />

  return (
    <ApiProvider>
      <RoleProvider availableGroups={availableRoles} isLoading={isResolvingRoles}>
        <UploadWrapper>
          <Layout />
        </UploadWrapper>
      </RoleProvider>
    </ApiProvider>
  )
}

function SignedOutRoute() {
  const { isAuthenticated, isLoading } = useAuth()
  if (isLoading) return null
  if (isAuthenticated) return <Navigate to="/" replace />
  return <LoginScreen />
}

function App() {
  useEffect(() => {
    document.title = `${displayName} | ${appConfig.branding.systemName}`

    if (appConfig.branding.favicon) {
      const link = (document.querySelector("link[rel~='icon']") as HTMLLinkElement) ?? document.createElement('link')
      link.rel = 'icon'
      link.href = appConfig.branding.favicon
      document.head.appendChild(link)
    }
  }, [])

  return (
    <Routes>
      {import.meta.env.DEV && <Route path="/dev/zones" element={<ZonePreviewScreen />} />}
      <Route path="/login" element={<SignedOutRoute />} />

      <Route element={<ProtectedLayout />}>
        <Route path="/" element={<Navigate to="/consignments" replace />} />
        <Route path="/consignments" element={<ConsignmentScreen />} />
        <Route path="/consignments/:consignmentId" element={<ConsignmentDetailScreen />} />
        <Route path="/consignments/:consignmentId/tasks/:taskId" element={<TaskDetailScreen />} />
      </Route>

      <Route path="*" element={<Navigate to="/login" replace />} />
    </Routes>
  )
}

export default App
