import { useState, useCallback } from 'react';
import { BrowserRouter, Routes, Route } from 'react-router-dom';
import Sidebar from './components/Sidebar';
import GroupsPage from './pages/GroupsPage';
import GroupDetailPage from './pages/GroupDetailPage';
import HotelsPage from './pages/HotelsPage';
import HotelEditPage from './pages/HotelEditPage';

export default function App() {
  const [sidebarOpen, setSidebarOpen] = useState(false);
  const closeSidebar = useCallback(() => setSidebarOpen(false), []);
  const toggleSidebar = useCallback(() => setSidebarOpen(o => !o), []);

  return (
    <BrowserRouter>
      <div className="app-layout">
        <Sidebar open={sidebarOpen} onClose={closeSidebar} />
        {sidebarOpen && (
          <div className="sidebar-backdrop" onClick={closeSidebar} />
        )}
        <main className="main-content">
          <button
            className="hamburger-btn"
            onClick={toggleSidebar}
            aria-label="Toggle menu"
          >
            <span className="hamburger-icon" />
            <span className="hamburger-icon" />
            <span className="hamburger-icon" />
          </button>
          <Routes>
            <Route path="/" element={<GroupsPage />} />
            <Route path="/groups/:id" element={<GroupDetailPage />} />
            <Route path="/hotels" element={<HotelsPage />} />
            <Route path="/hotels/:id" element={<HotelEditPage />} />
          </Routes>
        </main>
      </div>
    </BrowserRouter>
  );
}
