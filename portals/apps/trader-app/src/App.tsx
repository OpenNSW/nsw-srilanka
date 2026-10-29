import { type ReactNode } from 'react'
import { Routes, Route, Navigate } from 'react-router-dom'
import { Layout } from './components/Layout'
import { Footer } from '@/components/Layout'
import { ConsignmentScreen } from './features/consignment/screens/ConsignmentScreen.tsx'
import { ConsignmentDetailScreen } from './features/consignment/screens/ConsignmentDetailScreen.tsx'
import { TaskDetailScreen } from '@/features/task/TaskDetailScreen.tsx'
import { UnsavedChangesGuard } from '@/features/zone/components/UnsavedChangesGuard'
import { CaseDetailScreen, CaseListScreen } from '@/features/case'
import { isAgencyMode } from '@/features/case/mode'
import { ROUTES } from '@/constants'
import { AdminConsignmentEngineStatusScreen } from '@/features/admin/screens/AdminConsignmentEngineStatusScreen.tsx'
import { AdminConsignmentDetailScreen } from '@/features/admin/screens/AdminConsignmentDetailScreen.tsx'
import { useAuth } from 'react-oidc-context'
import { LoginScreen } from './screens/LoginScreen.tsx'
import { ProfileProvider } from './services/ProfileContext'
import { SearchServiceProvider, UploadProvider } from '@opennsw/jsonforms-renderers'
import { uploadFile, getDownloadUrl } from './services/storage'
import { searchServices } from './services/searchService'
import { useAuthContext } from './hooks/useAuthContext'
import { ZonePreviewScreen } from '@/features/zone/ZonePreviewScreen.tsx'
import { appConfig, displayName } from './config'
import { useEffect } from 'react'

function UploadWrapper({ children }: { children: ReactNode }) {
  return (
    <SearchServiceProvider services={searchServices}>
      <UploadProvider onUpload={(file) => uploadFile(file)} getDownloadUrl={(key) => getDownloadUrl(key)}>
        {children}
      </UploadProvider>
    </SearchServiceProvider>
  )
}

function ProtectedLayout() {
  const { isSignedIn, isLoading } = useAuthContext()

  if (isLoading) return null
  if (!isSignedIn) return <Navigate to="/login" replace />

  // Every signed-in user is admitted; the backend authorizes each request by the
  // token's roles.
  return (
    <ProfileProvider>
      <UploadWrapper>
        <Layout />
      </UploadWrapper>
    </ProfileProvider>
  )
}

function SignedOutRoute() {
  const { isAuthenticated, isLoading } = useAuth()
  if (isLoading) {
    return (
      <div className="min-h-screen flex items-center justify-center bg-app-bg">
        <div className="animate-spin rounded-full h-10 w-10 border-t-2 border-b-2 border-primary"></div>
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
    <>
      <Routes>
        {import.meta.env.DEV && <Route path="/dev/zones" element={<ZonePreviewScreen />} />}
        <Route path="/login" element={<SignedOutRoute />} />

        <Route element={<ProtectedLayout />}>
          <Route path="/" element={<Navigate to={ROUTES.CONSIGNMENTS} replace />} />
          <Route path={ROUTES.CONSIGNMENTS} element={isAgencyMode ? <CaseListScreen /> : <ConsignmentScreen />} />
          <Route
            path={ROUTES.CONSIGNMENT_DETAIL_PATTERN}
            element={isAgencyMode ? <CaseDetailScreen /> : <ConsignmentDetailScreen />}
          />
          <Route
            path={ROUTES.TASK_FORM_PATTERN}
            element={
              <UnsavedChangesGuard>
                <TaskDetailScreen />
              </UnsavedChangesGuard>
            }
          />
          {/* Internal ops view, not linked from nav — see AdminConsignmentEngineStatusScreen. */}
          <Route path="/admin/consignments/:consignmentId" element={<AdminConsignmentEngineStatusScreen />} />
          <Route path="/admin/consignments/:consignmentId/view" element={<AdminConsignmentDetailScreen />} />
        </Route>

        <Route path="*" element={<Navigate to="/login" replace />} />
      </Routes>

      <Footer />
    </>
  )
}

export default App
