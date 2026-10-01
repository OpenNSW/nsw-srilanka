import { Outlet } from 'react-router-dom'
import { TopBar } from './TopBar'

// The TopBar floats free of the viewport edge (see its own component)
// rather than sitting flush against it. GAP_PX is the margin that
// separates it from the edges; CONTENT_TOP_PX derives from it plus the
// bar's own height so the two stay in lockstep without duplicating magic
// numbers wherever content needs to clear the bar.
const GAP_PX = 12
const TOPBAR_HEIGHT_PX = 64
// Exported so screens that manage their own scroll container (rather than relying on
// <main>'s own height above) can clear the bar by the same amount instead of hardcoding it.
export const CONTENT_TOP_PX = TOPBAR_HEIGHT_PX + GAP_PX * 2

export function Layout() {
  return (
    <div className="min-h-screen bg-app-bg">
      <TopBar />

      <main
        style={{
          marginTop: `${CONTENT_TOP_PX}px`,
          minHeight: `calc(100vh - ${CONTENT_TOP_PX}px)`,
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
