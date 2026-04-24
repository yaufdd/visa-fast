import { NavLink } from 'react-router-dom';
import { useTheme } from '../theme';

const navItems = [
  { to: '/', label: 'Подачи', icon: '◫', end: true },
  { to: '/submissions', label: 'Анкеты', icon: '⌬', end: false },
  { to: '/hotels', label: 'Отели', icon: '⊞', end: true },
  { to: '/templates', label: 'Шаблоны', icon: '❡', end: true },
];

export default function Sidebar({ open, onClose }) {
  const { theme, toggleTheme } = useTheme();
  return (
    <aside className={`sidebar${open ? ' sidebar--open' : ''}`}>
      <div className="sidebar-logo">
        <span className="sidebar-logo-icon">⛩</span>
        <div>
          <div className="sidebar-logo-title">FujiTravel</div>
          <div className="sidebar-logo-sub">Admin Panel</div>
        </div>
      </div>

      <nav className="sidebar-nav">
        {navItems.map(item => (
          <NavLink
            key={item.to}
            to={item.to}
            end={item.end}
            className={({ isActive }) =>
              `sidebar-link${isActive ? ' active' : ''}`
            }
            onClick={onClose}
          >
            <span className="sidebar-link-icon">{item.icon}</span>
            {item.label}
          </NavLink>
        ))}
      </nav>

      <div className="sidebar-footer">
        <div className="sidebar-footer-row">
          <button
            type="button"
            className="sidebar-theme-toggle"
            onClick={toggleTheme}
            aria-label={theme === 'dark' ? 'Switch to light theme' : 'Switch to dark theme'}
            title={theme === 'dark' ? 'Светлая тема' : 'Тёмная тема'}
          >
            <span className="sidebar-theme-toggle-icon" key={theme}>
              {theme === 'dark' ? '☀' : '☾'}
            </span>
          </button>
        </div>
      </div>

      <style>{`
        .sidebar {
          width: 220px;
          min-width: 220px;
          background: var(--graphite);
          border-right: 1px solid var(--border);
          display: flex;
          flex-direction: column;
          height: 100vh; /* fallback */
          height: 100dvh;
          position: sticky;
          top: 0;
        }

        .sidebar-logo {
          display: flex;
          align-items: center;
          gap: 12px;
          padding: 24px 20px 20px;
          border-bottom: 1px solid var(--border);
        }

        .sidebar-logo-icon {
          font-size: 22px;
          line-height: 1;
        }

        .sidebar-logo-title {
          font-size: 15px;
          font-weight: 600;
          color: var(--white);
          letter-spacing: -0.2px;
        }

        .sidebar-logo-sub {
          font-size: 11px;
          color: var(--white-dim);
          margin-top: 1px;
          text-transform: uppercase;
          letter-spacing: 0.05em;
        }

        .sidebar-nav {
          flex: 1;
          padding: 20px 12px;
          display: flex;
          flex-direction: column;
          gap: 2px;
        }

        .sidebar-nav-label {
          font-size: 10px;
          font-weight: 600;
          text-transform: uppercase;
          letter-spacing: 0.1em;
          color: var(--white-dim);
          padding: 0 8px;
          margin-bottom: 8px;
          opacity: 0.6;
        }

        .sidebar-link {
          display: flex;
          align-items: center;
          gap: 10px;
          padding: 9px 10px;
          border-radius: 7px;
          color: var(--white-dim);
          font-size: 13px;
          font-weight: 500;
          transition: all 0.15s ease;
        }

        .sidebar-link:hover {
          background: var(--gray);
          color: var(--white);
        }

        .sidebar-link.active {
          background: var(--accent-dim);
          color: var(--accent);
        }

        .sidebar-link-icon {
          font-size: 15px;
          width: 18px;
          text-align: center;
          flex-shrink: 0;
        }

        .sidebar-footer {
          padding: 12px 16px 16px;
          border-top: 1px solid var(--border);
        }

        .sidebar-footer-row {
          display: flex;
          align-items: center;
          justify-content: space-between;
          gap: 8px;
        }

        .sidebar-theme-toggle {
          width: 32px;
          height: 32px;
          display: inline-flex;
          align-items: center;
          justify-content: center;
          padding: 0;
          border-radius: 7px;
          background: transparent;
          color: var(--white-dim);
          border: 1px solid var(--border);
          cursor: pointer;
          transition: color 0.15s ease, background 0.15s ease, border-color 0.15s ease, transform 0.15s ease;
          overflow: hidden;
        }

        .sidebar-theme-toggle:hover {
          color: var(--white);
          background: var(--gray);
          border-color: var(--white-dim);
        }

        .sidebar-theme-toggle:active {
          transform: scale(0.92);
        }

        .sidebar-theme-toggle-icon {
          font-size: 15px;
          line-height: 1;
          display: inline-block;
          transform-origin: center;
          animation: theme-icon-pop 0.35s cubic-bezier(0.34, 1.56, 0.64, 1);
        }

        @keyframes theme-icon-pop {
          0% {
            opacity: 0;
            transform: rotate(-180deg) scale(0.4);
          }
          60% {
            opacity: 1;
            transform: rotate(10deg) scale(1.15);
          }
          100% {
            opacity: 1;
            transform: rotate(0deg) scale(1);
          }
        }

        .sidebar-footer-text {
          font-size: 11px;
          color: var(--white-dim);
          opacity: 0.5;
          font-family: var(--font-mono);
        }

        @media (max-width: 767px) {
          .sidebar {
            position: fixed;
            top: 0;
            left: 0;
            z-index: 1000;
            height: 100vh; /* fallback */
            height: 100dvh;
            transform: translateX(-100%);
            transition: transform 0.25s ease;
          }
          .sidebar.sidebar--open {
            transform: translateX(0);
          }
        }
      `}</style>
    </aside>
  );
}
