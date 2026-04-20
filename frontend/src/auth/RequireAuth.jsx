import { Navigate, useLocation } from 'react-router-dom'
import { useAuth } from './AuthContext'

export default function RequireAuth({ children }) {
  const { loading, user } = useAuth()
  const location = useLocation()
  if (loading) return <div className="auth-loading">Загрузка…</div>
  if (!user) return <Navigate to="/login" state={{ from: location }} replace />
  return children
}
