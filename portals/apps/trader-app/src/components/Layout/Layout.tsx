import { Outlet } from 'react-router-dom'
import { Sidebar } from './Sidebar'
import { TopBar } from './TopBar'
import { useState } from 'react'

export function Layout() {
  const [isSidebarExpanded, setIsSidebarExpanded] = useState(() => {
    const savedState = localStorage.getItem('sidebarExpanded')
    // Default to true if no saved state is found
    return savedState !== null ? savedState === 'true' : true
  })

  const sidebarWidth = isSidebarExpanded ? 256 : 80 // w-64 = 256px, w-20 = 80px
  // Save sidebar state to localStorage when it changes
  const handleToggleSidebar = () => {
    setIsSidebarExpanded((prev) => {
      const newState = !prev
      localStorage.setItem('sidebarExpanded', String(newState))
      return newState
    })
  }

  // The TopBar and Sidebar both float free of the viewport edge (see their
  // own components) rather than sitting flush against it. GAP_PX is the
  // margin that separates them from the edges and from each other; every
  // offset below is derived from it plus the bar's own height so the two
  // stay in lockstep without duplicating magic numbers three times over.
  const GAP_PX = 12
  const TOPBAR_HEIGHT_PX = 64
  const CONTENT_TOP_PX = TOPBAR_HEIGHT_PX + GAP_PX * 2

  return (
    <div className="min-h-screen bg-app-bg">
      <TopBar />

      <div className="flex">
        <Sidebar isExpanded={isSidebarExpanded} onToggle={handleToggleSidebar} />

        <main
          style={{
            marginLeft: `${sidebarWidth + GAP_PX * 2}px`,
            width: `calc(100% - ${sidebarWidth + GAP_PX * 2}px)`,
            marginTop: `${CONTENT_TOP_PX}px`,
            minHeight: `calc(100vh - ${CONTENT_TOP_PX}px)`,
          }}
          className="transition-all duration-300 bg-app-bg pb-16 sm:pb-8"
        >
          <div className="max-w-7xl mx-auto">
            <Outlet />
          </div>
        </main>
      </div>
    </div>
  )
}
