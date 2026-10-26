import { Outlet } from 'react-router-dom'
import { TopBar } from './TopBar'

// Content clears the fixed TopBar by --app-top-offset (index.css), which
// follows the bar's own position: flush with the top on small screens,
// floating from 1024 px up. Screens that manage their own scroll container
// clear it by the same variable.
export function Layout() {
  return (
    <div className="min-h-screen bg-app-bg">
      <TopBar />

      <main
        style={{
          marginTop: 'var(--app-top-offset)',
          minHeight: 'calc(100vh - var(--app-top-offset))',
        }}
        className="bg-app-bg pb-16 sm:pb-8"
      >
        <div className="max-w-7xl mx-auto">
          <Outlet />
        </div>
      </main>
    </div>
  )
}
