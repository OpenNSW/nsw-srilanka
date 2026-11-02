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
import { AccessRestrictedScreen } from '@/screens/AccessRestrictedScreen'
import { RequireScope } from '@/components/RequireScope'
import { NAV_STRUCTURE, firstPermittedPath } from '@/navigation/navItems'
import { APP_ROUTES, type AppRoute } from '@/navigation/routes'
import { useScopes } from '@/hooks/useScopes'
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

  // Every signed-in user gets the layout; each route shows its screen only to a
  // user granted the scope that screen needs (see restrictTo).
  return (
    <ProfileProvider>
      <UploadWrapper>
        <Layout />
      </UploadWrapper>
    </ProfileProvider>
  )
}

// A route's screen, shown only to a user granted the scope the route needs.
function restrictTo(route: AppRoute, screen: ReactNode) {
  return (
    <RequireScope scope={route.scope} fallback={<AccessRestrictedScreen />}>
      {screen}
    </RequireScope>
  )
}

// Sends / to the first nav destination the user's scopes reach.
function HomeRedirect() {
  const { hasScope } = useScopes()
  const path = firstPermittedPath(NAV_STRUCTURE, hasScope)
  return path ? <Navigate to={path} replace /> : <AccessRestrictedScreen />
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
          <Route path="/" element={<HomeRedirect />} />
          <Route
            path={APP_ROUTES.consignments.path}
            element={restrictTo(APP_ROUTES.consignments, isAgencyMode ? <CaseListScreen /> : <ConsignmentScreen />)}
          />
          <Route
            path={APP_ROUTES.consignmentDetail.path}
            element={restrictTo(
              APP_ROUTES.consignmentDetail,
              isAgencyMode ? <CaseDetailScreen /> : <ConsignmentDetailScreen />,
            )}
          />
          <Route
            path={APP_ROUTES.task.path}
            element={restrictTo(
              APP_ROUTES.task,
              <UnsavedChangesGuard>
                <TaskDetailScreen />
              </UnsavedChangesGuard>,
            )}
          />
          {/* Internal ops view, not linked from nav — see AdminConsignmentEngineStatusScreen. */}
          <Route
            path={APP_ROUTES.adminConsignment.path}
            element={restrictTo(APP_ROUTES.adminConsignment, <AdminConsignmentEngineStatusScreen />)}
          />
          <Route
            path={APP_ROUTES.adminConsignmentView.path}
            element={restrictTo(APP_ROUTES.adminConsignmentView, <AdminConsignmentDetailScreen />)}
          />
        </Route>

        <Route path="*" element={<Navigate to="/login" replace />} />
      </Routes>

      <Footer />
    </>
  )
}

export default App
