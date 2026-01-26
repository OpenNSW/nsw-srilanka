import { Outlet } from 'react-router-dom'
import { Sidebar } from './Sidebar'
import { TopBar } from './TopBar'
import {useState} from "react";

export function Layout() {
  const [isSidebarExpanded, setIsSidebarExpanded] = useState(() => {
    const savedState = localStorage.getItem('sidebarExpanded');
    // Default to true if no saved state is found
    return savedState !== null ? savedState === 'true' : true;
  });
  const sidebarWidth = isSidebarExpanded ? 256 : 80; // w-64 = 256px, w-20 = 80px
  // Save sidebar state to localStorage when it changes
  const handleToggleSidebar = () => {
    setIsSidebarExpanded((prev) => {
      const newState = !prev;
      localStorage.setItem('sidebarExpanded', String(newState));
      return newState;
    });
  };

  return (
    <div className="min-h-screen bg-gray-50">
      {/* Top Bar - Full Width */}
      <TopBar />

      <div className="flex">

      <Sidebar isExpanded={isSidebarExpanded} onToggle={handleToggleSidebar} />

      {/* Main Content Area */}
      <main
        style={{ marginLeft: `${sidebarWidth}px`, width: `calc(100% - ${sidebarWidth}px)` }}
        className="min-h-[calc(100vh-64px)] transition-all duration-300 mt-16"
      >
          <Outlet />
      </main>
      </div>
    </div>
  )
}