import { useState, useCallback } from 'react';
import { BrowserRouter, Routes, Route } from 'react-router-dom';
import { AuthProvider, useAuth } from './auth/AuthContext';
import RequireAuth from './auth/RequireAuth';
import Sidebar from './components/Sidebar';
import GroupsPage from './pages/GroupsPage';
import GroupDetailPage from './pages/GroupDetailPage';
import HotelsPage from './pages/HotelsPage';
import HotelEditPage from './pages/HotelEditPage';
import TemplatesPage from './pages/TemplatesPage';
import SubmissionsListPage from './pages/SubmissionsListPage';
import SubmissionDetailPage from './pages/SubmissionDetailPage';
import SubmissionSettingsPage from './pages/SubmissionSettingsPage';
import ProfilePage from './pages/ProfilePage';
import TouristDetailPage from './pages/TouristDetailPage';
import SubmissionFormPage from './pages/SubmissionFormPage';
import FormThanksPage from './pages/FormThanksPage';
import ConsentPage from './pages/ConsentPage';
import LoginPage from './pages/LoginPage';
import RegisterPage from './pages/RegisterPage';
import PublicFormFallbackPage from './pages/PublicFormFallbackPage';

function AdminShellHeader({ onMenuClick }) {
  const { org } = useAuth();
  return (
    <header className="admin-shell-header">
      <button
        className="hamburger-btn"
        onClick={onMenuClick}
        aria-label="Открыть меню"
        type="button"
      >
        <span className="hamburger-icon" />
        <span className="hamburger-icon" />
        <span className="hamburger-icon" />
      </button>
      <div className="org-info">
        <strong>{org?.name}</strong>
        <span className="muted">/{org?.slug}</span>
      </div>
    </header>
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
        <AdminShellHeader onMenuClick={toggleSidebar} />
        {children}
      </main>
    </div>
  );
}

function AdminRoutes() {
  return (
    <AdminShell>
      <Routes>
        <Route path="/" element={<GroupsPage />} />
        <Route path="/groups/:id" element={<GroupDetailPage />} />
        <Route path="/hotels" element={<HotelsPage />} />
        <Route path="/hotels/:id" element={<HotelEditPage />} />
        <Route path="/templates" element={<TemplatesPage />} />
        <Route path="/submissions" element={<SubmissionsListPage />} />
        <Route path="/submissions/:id" element={<SubmissionDetailPage />} />
        <Route path="/submissions/:id/settings" element={<SubmissionSettingsPage />} />
        <Route path="/tourists/:id" element={<TouristDetailPage />} />
        <Route path="/profile" element={<ProfilePage />} />
      </Routes>
    </AdminShell>
  );
}

export default function App() {
  return (
    <AuthProvider>
      <BrowserRouter>
        <Routes>
          {/* Public */}
          <Route path="/login" element={<LoginPage />} />
          <Route path="/register" element={<RegisterPage />} />
          <Route path="/form/thanks" element={<FormThanksPage />} />
          <Route path="/form/:slug" element={<SubmissionFormPage />} />
          <Route path="/form" element={<PublicFormFallbackPage />} />
          <Route path="/consent" element={<ConsentPage />} />

          {/* Protected admin */}
          <Route
            path="/*"
            element={
              <RequireAuth>
                <AdminRoutes />
              </RequireAuth>
            }
          />
        </Routes>
      </BrowserRouter>
    </AuthProvider>
  );
}
