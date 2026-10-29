import { Outlet } from 'react-router-dom'
import { TopBar } from './TopBar'

// Content clears the fixed TopBar by --app-top-offset (index.css), which
// follows the bar's own position: flush with the top on small screens,
// floating from 1024 px up. Screens that manage their own scroll container
// clear it by the same variable. It's padding, not margin: a margin would
// collapse through the wrapper (the fixed TopBar takes no space), leaving the
// strip behind the bar painted by the Theme root instead of bg-app-bg.
export function Layout() {
  return (
    <div className="min-h-screen bg-app-bg">
      <TopBar />

      <main style={{ paddingTop: 'var(--app-top-offset)' }} className="min-h-screen bg-app-bg pb-16 sm:pb-8">
        <div className="max-w-7xl mx-auto">
          <Outlet />
        </div>
      </main>
    </div>
  )
}
