import { type ReactNode } from 'react'
import { Routes, Route, Navigate } from 'react-router-dom'
import './App.css'
import { Layout } from './components/Layout'
import { ConsignmentScreen } from './features/consignment/screens/ConsignmentScreen.tsx'
import { ConsignmentDetailScreen } from './features/consignment/screens/ConsignmentDetailScreen.tsx'
import { TaskDetailScreen } from '@/features/task/TaskDetailScreen.tsx'
import { useAuth } from 'react-oidc-context'
import { LoginScreen } from './screens/LoginScreen.tsx'
import { RoleProvider } from './services/RoleContext'
import { UploadProvider } from '@opennsw/jsonforms-renderers'
import { uploadFile, getDownloadUrl } from './services/storage'
import { useAuthContext } from './hooks/useAuthContext'
import { UnauthorizedScreen } from './screens/UnauthorizedScreen.tsx'
import { ZonePreviewScreen } from '@/features/zone/ZonePreviewScreen.tsx'
import { appConfig, displayName } from './config'
import { useEffect } from 'react'

function UploadWrapper({ children }: { children: ReactNode }) {
  return (
    <UploadProvider onUpload={(file) => uploadFile(file)} getDownloadUrl={(key) => getDownloadUrl(key)}>
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
    <RoleProvider availableGroups={availableRoles} isLoading={isResolvingRoles}>
      <UploadWrapper>
        <Layout />
      </UploadWrapper>
    </RoleProvider>
  )
}

function SignedOutRoute() {
  const { isAuthenticated, isLoading } = useAuth()
  if (isLoading) {
    return (
      <div className="min-h-screen flex items-center justify-center bg-app-bg">
        <div className="animate-spin rounded-full h-10 w-10 border-t-2 border-b-2 border-indigo-600"></div>
      </div>
    )
  }
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
