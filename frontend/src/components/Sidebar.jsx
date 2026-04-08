import { NavLink } from 'react-router-dom';

const navItems = [
  { to: '/', label: 'Группы', icon: '◫', end: true },
  { to: '/hotels', label: 'Отели', icon: '⊞', end: true },
];

export default function Sidebar() {
  return (
    <aside className="sidebar">
      <div className="sidebar-logo">
        <span className="sidebar-logo-icon">⛩</span>
        <div>
          <div className="sidebar-logo-title">FujiTravel</div>
          <div className="sidebar-logo-sub">Admin Panel</div>
        </div>
      </div>

      <nav className="sidebar-nav">
        <div className="sidebar-nav-label">Navigation</div>
        {navItems.map(item => (
          <NavLink
            key={item.to}
            to={item.to}
            end={item.end}
            className={({ isActive }) =>
              `sidebar-link${isActive ? ' active' : ''}`
            }
          >
            <span className="sidebar-link-icon">{item.icon}</span>
            {item.label}
          </NavLink>
        ))}
      </nav>

      <div className="sidebar-footer">
        <div className="sidebar-footer-text">v1.0.0</div>
      </div>

      <style>{`
        .sidebar {
          width: 220px;
          min-width: 220px;
          background: var(--graphite);
          border-right: 1px solid var(--border);
          display: flex;
          flex-direction: column;
          height: 100vh;
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
          padding: 16px 20px;
          border-top: 1px solid var(--border);
        }

        .sidebar-footer-text {
          font-size: 11px;
          color: var(--white-dim);
          opacity: 0.5;
          font-family: var(--font-mono);
        }
      `}</style>
    </aside>
  );
}
