import { useState, useCallback } from 'react';
import { BrowserRouter, Routes, Route, useLocation } from 'react-router-dom';
import Sidebar from './components/Sidebar';
import GroupsPage from './pages/GroupsPage';
import GroupDetailPage from './pages/GroupDetailPage';
import HotelsPage from './pages/HotelsPage';
import HotelEditPage from './pages/HotelEditPage';
import SubmissionsListPage from './pages/SubmissionsListPage';
import SubmissionDetailPage from './pages/SubmissionDetailPage';
import SubmissionFormPage from './pages/SubmissionFormPage';
import FormThanksPage from './pages/FormThanksPage';
import ConsentPage from './pages/ConsentPage';

// Routes that should render standalone (no admin chrome / sidebar).
const PUBLIC_ROUTE_PREFIXES = ['/form', '/consent'];

function isPublicPath(pathname) {
  return PUBLIC_ROUTE_PREFIXES.some(
    (p) => pathname === p || pathname.startsWith(`${p}/`)
  );
}

function AdminShell({ children }) {
  const [sidebarOpen, setSidebarOpen] = useState(false);
  const closeSidebar = useCallback(() => setSidebarOpen(false), []);
  const toggleSidebar = useCallback(() => setSidebarOpen((o) => !o), []);

  return (
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
        {children}
      </main>
    </div>
  );
}

function RootLayout() {
  const { pathname } = useLocation();
  const publicRoute = isPublicPath(pathname);

  const routes = (
    <Routes>
      {/* Public routes (standalone layout) */}
      <Route path="/form" element={<SubmissionFormPage />} />
      <Route path="/form/thanks" element={<FormThanksPage />} />
      <Route path="/consent" element={<ConsentPage />} />

      {/* Admin routes */}
      <Route path="/" element={<GroupsPage />} />
      <Route path="/groups/:id" element={<GroupDetailPage />} />
      <Route path="/hotels" element={<HotelsPage />} />
      <Route path="/hotels/:id" element={<HotelEditPage />} />
      <Route path="/submissions" element={<SubmissionsListPage />} />
      <Route path="/submissions/:id" element={<SubmissionDetailPage />} />
    </Routes>
  );

  if (publicRoute) {
    return <main className="public-shell">{routes}</main>;
  }
  return <AdminShell>{routes}</AdminShell>;
}

export default function App() {
  return (
    <BrowserRouter>
      <RootLayout />
    </BrowserRouter>
  );
}
