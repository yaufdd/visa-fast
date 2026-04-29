import { useAuth } from '../auth/AuthContext';

// Lightweight profile view. Shows the manager's email and the org they
// belong to, plus the "Выйти" button — moved here from the AdminShell
// header so the top bar stays compact on mobile.
export default function ProfilePage() {
  const { user, org, logout } = useAuth();

  return (
    <div className="page-container">
      <div className="page-header">
        <div>
          <div className="page-title">Профиль</div>
          <div className="page-subtitle" style={{ marginTop: 6 }}>
            Аккаунт менеджера и выход из системы
          </div>
        </div>
      </div>

      <div
        style={{
          padding: 20,
          border: '1px solid var(--border)',
          borderRadius: 10,
          background: 'var(--gray-dark)',
          display: 'flex',
          flexDirection: 'column',
          gap: 14,
          maxWidth: 480,
        }}
      >
        <div>
          <div
            style={{
              fontSize: 11,
              color: 'var(--white-dim)',
              textTransform: 'uppercase',
              letterSpacing: '0.05em',
              marginBottom: 4,
            }}
          >
            Email
          </div>
          <div style={{ fontSize: 14, color: 'var(--white)' }}>{user?.email || '—'}</div>
        </div>

        {org && (
          <div>
            <div
              style={{
                fontSize: 11,
                color: 'var(--white-dim)',
                textTransform: 'uppercase',
                letterSpacing: '0.05em',
                marginBottom: 4,
              }}
            >
              Турфирма
            </div>
            <div style={{ fontSize: 14, color: 'var(--white)' }}>
              {org.name}{' '}
              <span style={{ color: 'var(--white-dim)', fontFamily: 'var(--font-mono)', fontSize: 12 }}>
                /{org.slug}
              </span>
            </div>
          </div>
        )}

        <div style={{ marginTop: 4, display: 'flex', justifyContent: 'flex-start' }}>
          <button onClick={logout} className="btn btn-ghost">
            Выйти
          </button>
        </div>
      </div>
    </div>
  );
}
